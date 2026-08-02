package handler

import (
	"encoding/json"
	"testing"
)

func TestHandleWithdraw_PublishesCompletedAndDebitsWallet(t *testing.T) {
	env := setupHandlerTestEnv(t)

	seedDeposit(t, env, "60000000-0000-0000-0000-000000000001", testWalletA, 100)

	const requestID = "60000000-0000-0000-0000-000000000002"
	payload, _ := json.Marshal(WithdrawRequest{
		RequestID: requestID,
		WalletID:  testWalletA,
		Amount:    35,
		Currency:  "USD",
	})

	if err := env.client.Conn.Publish("wallet.withdraw", payload); err != nil {
		t.Fatalf("publish withdraw: %v", err)
	}

	event := env.waitForEvent(t, "wallet.events.completed")
	if event.RequestID != requestID || event.Operation != "withdraw" {
		t.Fatalf("unexpected event: %+v", event)
	}

	if balance := env.walletBalance(t, testWalletA); balance != 65 {
		t.Fatalf("wallet balance = %v, want 65", balance)
	}
}

func TestHandleWithdraw_InsufficientFundsPublishesFailed(t *testing.T) {
	env := setupHandlerTestEnv(t)

	seedDeposit(t, env, "60000000-0000-0000-0000-000000000003", testWalletA, 20)

	payload, _ := json.Marshal(WithdrawRequest{
		RequestID: "60000000-0000-0000-0000-000000000004",
		WalletID:  testWalletA,
		Amount:    50,
		Currency:  "USD",
	})

	if err := env.client.Conn.Publish("wallet.withdraw", payload); err != nil {
		t.Fatalf("publish withdraw: %v", err)
	}

	event := env.waitForEvent(t, "wallet.events.failed")
	if event.Status != "failed" {
		t.Fatalf("event status = %q, want failed", event.Status)
	}
	if event.Reason == nil || *event.Reason == "" {
		t.Fatal("expected failure reason")
	}

	if balance := env.walletBalance(t, testWalletA); balance != 20 {
		t.Fatalf("wallet balance = %v, want 20 (unchanged)", balance)
	}
}

func TestHandleWithdraw_DuplicateRequestIsIdempotent(t *testing.T) {
	env := setupHandlerTestEnv(t)

	seedDeposit(t, env, "60000000-0000-0000-0000-000000000005", testWalletA, 100)

	const requestID = "60000000-0000-0000-0000-000000000006"
	payload, _ := json.Marshal(WithdrawRequest{
		RequestID: requestID,
		WalletID:  testWalletA,
		Amount:    25,
		Currency:  "USD",
	})

	if err := env.client.Conn.Publish("wallet.withdraw", payload); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	env.waitForEvent(t, "wallet.events.completed")

	if err := env.client.Conn.Publish("wallet.withdraw", payload); err != nil {
		t.Fatalf("duplicate publish: %v", err)
	}
	env.waitForEvent(t, "wallet.events.completed")

	if balance := env.walletBalance(t, testWalletA); balance != 75 {
		t.Fatalf("wallet balance = %v, want 75", balance)
	}
}
