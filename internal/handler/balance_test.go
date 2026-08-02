package handler

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHandleBalance_ReturnsWalletBalance(t *testing.T) {
	env := setupHandlerTestEnv(t)

	seedDeposit(t, env, "80000000-0000-0000-0000-000000000001", testWalletA, 150)

	payload, _ := json.Marshal(BalanceRequest{WalletID: testWalletA})
	respMsg, err := env.conn.Request("wallet.balance", payload, 3*time.Second)
	if err != nil {
		t.Fatalf("balance request: %v", err)
	}

	var resp BalanceResponse
	if err := json.Unmarshal(respMsg.Data, &resp); err != nil {
		t.Fatalf("unmarshal balance response: %v", err)
	}

	if resp.WalletID != testWalletA {
		t.Fatalf("wallet_id = %q, want %q", resp.WalletID, testWalletA)
	}
	if resp.Balance != 150 {
		t.Fatalf("balance = %v, want 150", resp.Balance)
	}
	if resp.Currency != "USD" {
		t.Fatalf("currency = %q, want USD", resp.Currency)
	}
}

func TestHandleBalance_AfterWithdrawReflectsUpdatedBalance(t *testing.T) {
	env := setupHandlerTestEnv(t)

	seedDeposit(t, env, "80000000-0000-0000-0000-000000000002", testWalletA, 100)

	withdrawPayload, _ := json.Marshal(WithdrawRequest{
		RequestID: "80000000-0000-0000-0000-000000000003",
		WalletID:  testWalletA,
		Amount:    40,
		Currency:  "USD",
	})
	if err := env.client.Conn.Publish("wallet.withdraw", withdrawPayload); err != nil {
		t.Fatalf("publish withdraw: %v", err)
	}
	env.waitForEvent(t, "wallet.events.completed")

	balancePayload, _ := json.Marshal(BalanceRequest{WalletID: testWalletA})
	respMsg, err := env.conn.Request("wallet.balance", balancePayload, 3*time.Second)
	if err != nil {
		t.Fatalf("balance request: %v", err)
	}

	var resp BalanceResponse
	if err := json.Unmarshal(respMsg.Data, &resp); err != nil {
		t.Fatalf("unmarshal balance response: %v", err)
	}
	if resp.Balance != 60 {
		t.Fatalf("balance = %v, want 60", resp.Balance)
	}
}

func TestHandleBalance_NonExistentWalletReturnsNoResponse(t *testing.T) {
	env := setupHandlerTestEnv(t)

	payload, _ := json.Marshal(BalanceRequest{WalletID: "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"})
	_, err := env.conn.Request("wallet.balance", payload, 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout/no response for non-existent wallet")
	}
}
