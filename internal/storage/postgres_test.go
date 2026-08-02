package storage

import (
	"errors"
	"testing"
)

func TestDeposit_CreatesWalletWithBalance(t *testing.T) {
	store := setupTestStore(t)

	const requestID = "10000000-0000-0000-0000-000000000001"

	if err := store.Deposit(requestID, testWalletA, 100); err != nil {
		t.Fatalf("deposit: %v", err)
	}

	balance, currency, err := store.GetWalletBalance(testWalletA)
	if err != nil {
		t.Fatalf("get wallet balance: %v", err)
	}
	if balance != 100 {
		t.Fatalf("balance = %v, want 100", balance)
	}
	if currency != "USD" {
		t.Fatalf("currency = %q, want USD", currency)
	}
	if count := countTransactionsByRequestID(t, store, requestID); count != 1 {
		t.Fatalf("transaction count = %d, want 1", count)
	}
}

func TestDeposit_AccumulatesOnExistingWallet(t *testing.T) {
	store := setupTestStore(t)

	mustDeposit(t, store, "10000000-0000-0000-0000-000000000002", testWalletA, 100)
	if err := store.Deposit("10000000-0000-0000-0000-000000000003", testWalletA, 50); err != nil {
		t.Fatalf("second deposit: %v", err)
	}

	balance, _, err := store.GetWalletBalance(testWalletA)
	if err != nil {
		t.Fatalf("get wallet balance: %v", err)
	}
	if balance != 150 {
		t.Fatalf("balance = %v, want 150", balance)
	}
}

func TestDeposit_IdempotentDuplicateRequestID(t *testing.T) {
	store := setupTestStore(t)

	const requestID = "10000000-0000-0000-0000-000000000004"

	if err := store.Deposit(requestID, testWalletA, 100); err != nil {
		t.Fatalf("first deposit: %v", err)
	}

	err := store.Deposit(requestID, testWalletA, 100)
	if !errors.Is(err, ErrAlreadyProcessed) {
		t.Fatalf("second deposit error = %v, want ErrAlreadyProcessed", err)
	}

	balance, _, err := store.GetWalletBalance(testWalletA)
	if err != nil {
		t.Fatalf("get wallet balance: %v", err)
	}
	if balance != 100 {
		t.Fatalf("balance = %v, want 100 (no double credit)", balance)
	}
	if count := countTransactionsByRequestID(t, store, requestID); count != 1 {
		t.Fatalf("transaction count = %d, want 1", count)
	}
}

func TestDeposit_LedgerMatchesWalletBalance(t *testing.T) {
	store := setupTestStore(t)

	mustDeposit(t, store, "10000000-0000-0000-0000-000000000005", testWalletA, 100)
	mustDeposit(t, store, "10000000-0000-0000-0000-000000000006", testWalletA, 25)

	walletBalance, _, err := store.GetWalletBalance(testWalletA)
	if err != nil {
		t.Fatalf("get wallet balance: %v", err)
	}
	ledgerBalance, err := store.SumBalanceFromTransactions(testWalletA)
	if err != nil {
		t.Fatalf("sum ledger balance: %v", err)
	}
	if walletBalance != ledgerBalance {
		t.Fatalf("wallet balance %v != ledger balance %v", walletBalance, ledgerBalance)
	}
	if walletBalance != 125 {
		t.Fatalf("balance = %v, want 125", walletBalance)
	}
}

func TestWithdraw_DeductsBalance(t *testing.T) {
	store := setupTestStore(t)

	mustDeposit(t, store, "20000000-0000-0000-0000-000000000001", testWalletA, 100)

	const requestID = "20000000-0000-0000-0000-000000000002"
	if err := store.Withdraw(requestID, testWalletA, 30); err != nil {
		t.Fatalf("withdraw: %v", err)
	}

	balance, _, err := store.GetWalletBalance(testWalletA)
	if err != nil {
		t.Fatalf("get wallet balance: %v", err)
	}
	if balance != 70 {
		t.Fatalf("balance = %v, want 70", balance)
	}
	if count := countTransactionsByRequestID(t, store, requestID); count != 1 {
		t.Fatalf("transaction count = %d, want 1", count)
	}
}

func TestWithdraw_InsufficientFunds(t *testing.T) {
	store := setupTestStore(t)

	mustDeposit(t, store, "20000000-0000-0000-0000-000000000003", testWalletA, 50)

	err := store.Withdraw("20000000-0000-0000-0000-000000000004", testWalletA, 80)
	if err == nil {
		t.Fatal("expected insufficient funds error")
	}

	balance, _, err := store.GetWalletBalance(testWalletA)
	if err != nil {
		t.Fatalf("get wallet balance: %v", err)
	}
	if balance != 50 {
		t.Fatalf("balance = %v, want 50 (unchanged)", balance)
	}
	if count := countTransactions(t, store); count != 1 {
		t.Fatalf("transaction count = %d, want 1 (only deposit)", count)
	}
}

func TestWithdraw_NonExistentWallet(t *testing.T) {
	store := setupTestStore(t)

	err := store.Withdraw("20000000-0000-0000-0000-000000000005", testWalletC, 10)
	if err == nil {
		t.Fatal("expected error for non-existent wallet")
	}
	if count := countTransactions(t, store); count != 0 {
		t.Fatalf("transaction count = %d, want 0", count)
	}
}

func TestWithdraw_IdempotentDuplicateRequestID(t *testing.T) {
	store := setupTestStore(t)

	mustDeposit(t, store, "20000000-0000-0000-0000-000000000006", testWalletA, 100)

	const requestID = "20000000-0000-0000-0000-000000000007"
	if err := store.Withdraw(requestID, testWalletA, 20); err != nil {
		t.Fatalf("first withdraw: %v", err)
	}

	err := store.Withdraw(requestID, testWalletA, 20)
	if !errors.Is(err, ErrAlreadyProcessed) {
		t.Fatalf("second withdraw error = %v, want ErrAlreadyProcessed", err)
	}

	balance, _, err := store.GetWalletBalance(testWalletA)
	if err != nil {
		t.Fatalf("get wallet balance: %v", err)
	}
	if balance != 80 {
		t.Fatalf("balance = %v, want 80 (no double debit)", balance)
	}
}

func TestTransfer_MovesFundsBetweenWallets(t *testing.T) {
	store := setupTestStore(t)

	mustDeposit(t, store, "30000000-0000-0000-0000-000000000001", testWalletA, 100)
	mustDeposit(t, store, "30000000-0000-0000-0000-000000000002", testWalletB, 40)

	const requestID = "30000000-0000-0000-0000-000000000003"
	if err := store.Transfer(requestID, testWalletA, testWalletB, 25); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	balanceA, _, err := store.GetWalletBalance(testWalletA)
	if err != nil {
		t.Fatalf("get wallet A balance: %v", err)
	}
	balanceB, _, err := store.GetWalletBalance(testWalletB)
	if err != nil {
		t.Fatalf("get wallet B balance: %v", err)
	}

	if balanceA != 75 {
		t.Fatalf("wallet A balance = %v, want 75", balanceA)
	}
	if balanceB != 65 {
		t.Fatalf("wallet B balance = %v, want 65", balanceB)
	}
	if count := countTransactionsByRequestID(t, store, requestID); count != 1 {
		t.Fatalf("transaction count = %d, want 1", count)
	}
}

func TestTransfer_InsufficientFunds(t *testing.T) {
	store := setupTestStore(t)

	mustDeposit(t, store, "30000000-0000-0000-0000-000000000004", testWalletA, 30)
	mustDeposit(t, store, "30000000-0000-0000-0000-000000000005", testWalletB, 10)

	err := store.Transfer("30000000-0000-0000-0000-000000000006", testWalletA, testWalletB, 50)
	if err == nil {
		t.Fatal("expected insufficient funds error")
	}

	balanceA, _, _ := store.GetWalletBalance(testWalletA)
	balanceB, _, _ := store.GetWalletBalance(testWalletB)
	if balanceA != 30 || balanceB != 10 {
		t.Fatalf("balances changed on failed transfer: A=%v B=%v", balanceA, balanceB)
	}
	if count := countTransactions(t, store); count != 2 {
		t.Fatalf("transaction count = %d, want 2 (deposits only)", count)
	}
}

func TestTransfer_IdempotentDuplicateRequestID(t *testing.T) {
	store := setupTestStore(t)

	mustDeposit(t, store, "30000000-0000-0000-0000-000000000007", testWalletA, 100)
	mustDeposit(t, store, "30000000-0000-0000-0000-000000000008", testWalletB, 50)

	const requestID = "30000000-0000-0000-0000-000000000010"
	if err := store.Transfer(requestID, testWalletA, testWalletB, 20); err != nil {
		t.Fatalf("first transfer: %v", err)
	}

	err := store.Transfer(requestID, testWalletA, testWalletB, 20)
	if !errors.Is(err, ErrAlreadyProcessed) {
		t.Fatalf("second transfer error = %v, want ErrAlreadyProcessed", err)
	}

	balanceA, _, _ := store.GetWalletBalance(testWalletA)
	balanceB, _, _ := store.GetWalletBalance(testWalletB)
	if balanceA != 80 || balanceB != 70 {
		t.Fatalf("balances after duplicate transfer: A=%v B=%v, want A=80 B=70", balanceA, balanceB)
	}
}

func TestTransfer_LedgerMatchesWalletBalances(t *testing.T) {
	store := setupTestStore(t)

	mustDeposit(t, store, "30000000-0000-0000-0000-000000000011", testWalletA, 100)
	mustDeposit(t, store, "30000000-0000-0000-0000-000000000012", testWalletB, 50)
	if err := store.Transfer("30000000-0000-0000-0000-000000000013", testWalletA, testWalletB, 15); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	for _, walletID := range []string{testWalletA, testWalletB} {
		walletBalance, _, err := store.GetWalletBalance(walletID)
		if err != nil {
			t.Fatalf("get wallet balance %s: %v", walletID, err)
		}
		ledgerBalance, err := store.SumBalanceFromTransactions(walletID)
		if err != nil {
			t.Fatalf("sum ledger balance %s: %v", walletID, err)
		}
		if walletBalance != ledgerBalance {
			t.Fatalf("wallet %s: wallet balance %v != ledger balance %v", walletID, walletBalance, ledgerBalance)
		}
	}
}

func TestTransfer_NonExistentReceiverDebitsSender(t *testing.T) {
	store := setupTestStore(t)

	mustDeposit(t, store, "30000000-0000-0000-0000-000000000014", testWalletA, 100)

	nonExistent := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	err := store.Transfer("30000000-0000-0000-0000-000000000015", testWalletA, nonExistent, 40)
	if err != nil {
		t.Fatalf("transfer returned error (current behavior allows silent credit failure): %v", err)
	}

	balanceA, _, err := store.GetWalletBalance(testWalletA)
	if err != nil {
		t.Fatalf("get wallet A balance: %v", err)
	}
	if balanceA != 60 {
		t.Fatalf("wallet A balance = %v, want 60 (sender debited)", balanceA)
	}

	_, _, err = store.GetWalletBalance(nonExistent)
	if err == nil {
		t.Fatal("expected receiver wallet to not exist")
	}
}

func TestFullFlow_DepositWithdrawTransferBalance(t *testing.T) {
	store := setupTestStore(t)

	mustDeposit(t, store, "40000000-0000-0000-0000-000000000001", testWalletA, 200)
	if err := store.Withdraw("40000000-0000-0000-0000-000000000002", testWalletA, 50); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	mustDeposit(t, store, "40000000-0000-0000-0000-000000000003", testWalletB, 25)
	if err := store.Transfer("40000000-0000-0000-0000-000000000004", testWalletA, testWalletB, 75); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	balanceA, _, err := store.GetWalletBalance(testWalletA)
	if err != nil {
		t.Fatalf("get wallet A balance: %v", err)
	}
	balanceB, _, err := store.GetWalletBalance(testWalletB)
	if err != nil {
		t.Fatalf("get wallet B balance: %v", err)
	}

	if balanceA != 75 {
		t.Fatalf("wallet A balance = %v, want 75", balanceA)
	}
	if balanceB != 100 {
		t.Fatalf("wallet B balance = %v, want 100", balanceB)
	}
	if count := countTransactions(t, store); count != 4 {
		t.Fatalf("transaction count = %d, want 4", count)
	}
}
