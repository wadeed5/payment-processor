package handler

import (
	"encoding/json"
	"testing"
	"time"
)

// Integration tests exercise the full NATS path:
// subscribe handlers -> publish command -> storage processing -> publish outcome event.

func TestIntegration_Deposit_CompletedFlow(t *testing.T) {
	env := setupHandlerTestEnv(t)
	events := env.prepareEventListeners(t)

	const requestID = "a1000000-0000-0000-0000-000000000001"
	payload, err := json.Marshal(DepositRequest{
		RequestID: requestID,
		WalletID:  testWalletA,
		Amount:    250,
		Currency:  "USD",
	})
	if err != nil {
		t.Fatalf("marshal deposit: %v", err)
	}

	if err := env.client.Conn.Publish("wallet.deposit", payload); err != nil {
		t.Fatalf("publish deposit: %v", err)
	}

	events.expectCompleted(t, requestID, "deposit")

	if balance := env.walletBalance(t, testWalletA); balance != 250 {
		t.Fatalf("wallet balance = %v, want 250", balance)
	}
	if count := ledgerCount(t, env); count != 1 {
		t.Fatalf("ledger rows = %d, want 1", count)
	}
}

func TestIntegration_Deposit_DuplicatePublishesCompletedWithoutDoubleCredit(t *testing.T) {
	env := setupHandlerTestEnv(t)

	const requestID = "a1000000-0000-0000-0000-000000000002"
	payload, _ := json.Marshal(DepositRequest{
		RequestID: requestID,
		WalletID:  testWalletA,
		Amount:    100,
		Currency:  "USD",
	})

	firstEvents := env.prepareEventListeners(t)
	if err := env.client.Conn.Publish("wallet.deposit", payload); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	firstEvents.expectCompleted(t, requestID, "deposit")

	secondEvents := env.prepareEventListeners(t)
	if err := env.client.Conn.Publish("wallet.deposit", payload); err != nil {
		t.Fatalf("duplicate publish: %v", err)
	}
	secondEvents.expectCompleted(t, requestID, "deposit")

	if balance := env.walletBalance(t, testWalletA); balance != 100 {
		t.Fatalf("wallet balance = %v, want 100", balance)
	}
	if count := ledgerCount(t, env); count != 1 {
		t.Fatalf("ledger rows = %d, want 1", count)
	}
}

func TestIntegration_Deposit_InvalidPayloadPublishesNoOutcome(t *testing.T) {
	env := setupHandlerTestEnv(t)
	events := env.prepareEventListeners(t)

	if err := env.client.Conn.Publish("wallet.deposit", []byte("{not-json")); err != nil {
		t.Fatalf("publish invalid deposit: %v", err)
	}

	events.expectNoEvents(t, 500*time.Millisecond)
	if count := ledgerCount(t, env); count != 0 {
		t.Fatalf("ledger rows = %d, want 0", count)
	}
}

func TestIntegration_Withdraw_CompletedFlow(t *testing.T) {
	env := setupHandlerTestEnv(t)
	seedDeposit(t, env, "a2000000-0000-0000-0000-000000000001", testWalletA, 200)

	events := env.prepareEventListeners(t)
	const requestID = "a2000000-0000-0000-0000-000000000002"
	payload, _ := json.Marshal(WithdrawRequest{
		RequestID: requestID,
		WalletID:  testWalletA,
		Amount:    75,
		Currency:  "USD",
	})

	if err := env.client.Conn.Publish("wallet.withdraw", payload); err != nil {
		t.Fatalf("publish withdraw: %v", err)
	}

	events.expectCompleted(t, requestID, "withdraw")

	if balance := env.walletBalance(t, testWalletA); balance != 125 {
		t.Fatalf("wallet balance = %v, want 125", balance)
	}
}

func TestIntegration_Withdraw_InsufficientFundsPublishesFailed(t *testing.T) {
	env := setupHandlerTestEnv(t)
	seedDeposit(t, env, "a2000000-0000-0000-0000-000000000003", testWalletA, 40)

	events := env.prepareEventListeners(t)
	const requestID = "a2000000-0000-0000-0000-000000000004"
	payload, _ := json.Marshal(WithdrawRequest{
		RequestID: requestID,
		WalletID:  testWalletA,
		Amount:    100,
		Currency:  "USD",
	})

	if err := env.client.Conn.Publish("wallet.withdraw", payload); err != nil {
		t.Fatalf("publish withdraw: %v", err)
	}

	events.expectFailed(t, requestID, "withdraw", "insufficient funds")

	if balance := env.walletBalance(t, testWalletA); balance != 40 {
		t.Fatalf("wallet balance = %v, want 40 (unchanged)", balance)
	}
	if count := ledgerCount(t, env); count != 1 {
		t.Fatalf("ledger rows = %d, want 1 (deposit only)", count)
	}
}

func TestIntegration_Withdraw_NonExistentWalletPublishesFailed(t *testing.T) {
	env := setupHandlerTestEnv(t)
	events := env.prepareEventListeners(t)

	const requestID = "a2000000-0000-0000-0000-000000000005"
	payload, _ := json.Marshal(WithdrawRequest{
		RequestID: requestID,
		WalletID:  "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee",
		Amount:    10,
		Currency:  "USD",
	})

	if err := env.client.Conn.Publish("wallet.withdraw", payload); err != nil {
		t.Fatalf("publish withdraw: %v", err)
	}

	events.expectFailed(t, requestID, "withdraw", "")

	if count := ledgerCount(t, env); count != 0 {
		t.Fatalf("ledger rows = %d, want 0", count)
	}
}

func TestIntegration_Withdraw_InvalidPayloadPublishesNoOutcome(t *testing.T) {
	env := setupHandlerTestEnv(t)
	events := env.prepareEventListeners(t)

	if err := env.client.Conn.Publish("wallet.withdraw", []byte("[]")); err != nil {
		t.Fatalf("publish invalid withdraw: %v", err)
	}

	events.expectNoEvents(t, 500*time.Millisecond)
}

func TestIntegration_Transfer_CompletedFlow(t *testing.T) {
	env := setupHandlerTestEnv(t)
	seedDeposit(t, env, "a3000000-0000-0000-0000-000000000001", testWalletA, 300)
	seedDeposit(t, env, "a3000000-0000-0000-0000-000000000002", testWalletB, 50)

	events := env.prepareEventListeners(t)
	const requestID = "a3000000-0000-0000-0000-000000000003"
	payload, _ := json.Marshal(TransferRequest{
		RequestID:    requestID,
		FromWalletID: testWalletA,
		ToWalletID:   testWalletB,
		Amount:       120,
		Currency:     "USD",
	})

	if err := env.client.Conn.Publish("wallet.transfer", payload); err != nil {
		t.Fatalf("publish transfer: %v", err)
	}

	events.expectCompleted(t, requestID, "transfer")

	if balanceA := env.walletBalance(t, testWalletA); balanceA != 180 {
		t.Fatalf("wallet A balance = %v, want 180", balanceA)
	}
	if balanceB := env.walletBalance(t, testWalletB); balanceB != 170 {
		t.Fatalf("wallet B balance = %v, want 170", balanceB)
	}
}

func TestIntegration_Transfer_InsufficientFundsPublishesFailed(t *testing.T) {
	env := setupHandlerTestEnv(t)
	seedDeposit(t, env, "a3000000-0000-0000-0000-000000000004", testWalletA, 30)
	seedDeposit(t, env, "a3000000-0000-0000-0000-000000000005", testWalletB, 10)

	events := env.prepareEventListeners(t)
	const requestID = "a3000000-0000-0000-0000-000000000006"
	payload, _ := json.Marshal(TransferRequest{
		RequestID:    requestID,
		FromWalletID: testWalletA,
		ToWalletID:   testWalletB,
		Amount:       100,
		Currency:     "USD",
	})

	if err := env.client.Conn.Publish("wallet.transfer", payload); err != nil {
		t.Fatalf("publish transfer: %v", err)
	}

	events.expectFailed(t, requestID, "transfer", "insufficient funds")

	if balanceA := env.walletBalance(t, testWalletA); balanceA != 30 {
		t.Fatalf("wallet A balance = %v, want 30", balanceA)
	}
	if balanceB := env.walletBalance(t, testWalletB); balanceB != 10 {
		t.Fatalf("wallet B balance = %v, want 10", balanceB)
	}
}

func TestIntegration_Transfer_DuplicatePublishesCompletedWithoutDoubleMove(t *testing.T) {
	env := setupHandlerTestEnv(t)
	seedDeposit(t, env, "a3000000-0000-0000-0000-000000000007", testWalletA, 200)
	seedDeposit(t, env, "a3000000-0000-0000-0000-000000000008", testWalletB, 50)

	const requestID = "a3000000-0000-0000-0000-000000000009"
	payload, _ := json.Marshal(TransferRequest{
		RequestID:    requestID,
		FromWalletID: testWalletA,
		ToWalletID:   testWalletB,
		Amount:       40,
		Currency:     "USD",
	})

	firstEvents := env.prepareEventListeners(t)
	if err := env.client.Conn.Publish("wallet.transfer", payload); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	firstEvents.expectCompleted(t, requestID, "transfer")

	secondEvents := env.prepareEventListeners(t)
	if err := env.client.Conn.Publish("wallet.transfer", payload); err != nil {
		t.Fatalf("duplicate publish: %v", err)
	}
	secondEvents.expectCompleted(t, requestID, "transfer")

	if balanceA := env.walletBalance(t, testWalletA); balanceA != 160 {
		t.Fatalf("wallet A balance = %v, want 160", balanceA)
	}
	if balanceB := env.walletBalance(t, testWalletB); balanceB != 90 {
		t.Fatalf("wallet B balance = %v, want 90", balanceB)
	}
}

func TestIntegration_Transfer_InvalidPayloadPublishesNoOutcome(t *testing.T) {
	env := setupHandlerTestEnv(t)
	events := env.prepareEventListeners(t)

	if err := env.client.Conn.Publish("wallet.transfer", []byte("{invalid-json")); err != nil {
		t.Fatalf("publish invalid transfer: %v", err)
	}

	events.expectNoEvents(t, 500*time.Millisecond)
}

func TestIntegration_Balance_RequestReplyAfterDeposit(t *testing.T) {
	env := setupHandlerTestEnv(t)
	seedDeposit(t, env, "a4000000-0000-0000-0000-000000000001", testWalletA, 175)

	resp := env.requestBalance(t, testWalletA)
	if resp.WalletID != testWalletA {
		t.Fatalf("wallet_id = %q, want %q", resp.WalletID, testWalletA)
	}
	if resp.Balance != 175 {
		t.Fatalf("balance = %v, want 175", resp.Balance)
	}
	if resp.Currency != "USD" {
		t.Fatalf("currency = %q, want USD", resp.Currency)
	}
}

func TestIntegration_Balance_NonExistentWalletReturnsNoReply(t *testing.T) {
	env := setupHandlerTestEnv(t)

	payload, _ := json.Marshal(BalanceRequest{WalletID: "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"})
	_, err := env.conn.Request("wallet.balance", payload, 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected no reply for non-existent wallet")
	}
}

func TestIntegration_FullLifecycle_EndToEnd(t *testing.T) {
	env := setupHandlerTestEnv(t)

	// 1) Deposit wallet A
	depositEvents := env.prepareEventListeners(t)
	const depositID = "a5000000-0000-0000-0000-000000000001"
	depositPayload, _ := json.Marshal(DepositRequest{
		RequestID: depositID,
		WalletID:  testWalletA,
		Amount:    500,
		Currency:  "USD",
	})
	if err := env.client.Conn.Publish("wallet.deposit", depositPayload); err != nil {
		t.Fatalf("publish deposit: %v", err)
	}
	depositEvents.expectCompleted(t, depositID, "deposit")

	// 2) Balance check via request-reply
	if resp := env.requestBalance(t, testWalletA); resp.Balance != 500 {
		t.Fatalf("balance after deposit = %v, want 500", resp.Balance)
	}

	// 3) Withdraw from wallet A
	withdrawEvents := env.prepareEventListeners(t)
	const withdrawID = "a5000000-0000-0000-0000-000000000002"
	withdrawPayload, _ := json.Marshal(WithdrawRequest{
		RequestID: withdrawID,
		WalletID:  testWalletA,
		Amount:    100,
		Currency:  "USD",
	})
	if err := env.client.Conn.Publish("wallet.withdraw", withdrawPayload); err != nil {
		t.Fatalf("publish withdraw: %v", err)
	}
	withdrawEvents.expectCompleted(t, withdrawID, "withdraw")

	// 4) Fund wallet B
	seedDeposit(t, env, "a5000000-0000-0000-0000-000000000003", testWalletB, 25)

	// 5) Transfer A -> B
	transferEvents := env.prepareEventListeners(t)
	const transferID = "a5000000-0000-0000-0000-000000000004"
	transferPayload, _ := json.Marshal(TransferRequest{
		RequestID:    transferID,
		FromWalletID: testWalletA,
		ToWalletID:   testWalletB,
		Amount:       150,
		Currency:     "USD",
	})
	if err := env.client.Conn.Publish("wallet.transfer", transferPayload); err != nil {
		t.Fatalf("publish transfer: %v", err)
	}
	transferEvents.expectCompleted(t, transferID, "transfer")

	// 6) Final balances through request-reply
	respA := env.requestBalance(t, testWalletA)
	respB := env.requestBalance(t, testWalletB)
	if respA.Balance != 250 {
		t.Fatalf("final wallet A balance = %v, want 250", respA.Balance)
	}
	if respB.Balance != 175 {
		t.Fatalf("final wallet B balance = %v, want 175", respB.Balance)
	}
	if count := ledgerCount(t, env); count != 4 {
		t.Fatalf("ledger rows = %d, want 4", count)
	}
}
