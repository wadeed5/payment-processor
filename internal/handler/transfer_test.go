package handler

import (
	"encoding/json"
	"testing"
)

func TestHandleTransfer_PublishesCompletedAndMovesFunds(t *testing.T) {
	env := setupHandlerTestEnv(t)

	seedDeposit(t, env, "70000000-0000-0000-0000-000000000001", testWalletA, 100)
	seedDeposit(t, env, "70000000-0000-0000-0000-000000000002", testWalletB, 40)

	const requestID = "70000000-0000-0000-0000-000000000003"
	payload, _ := json.Marshal(TransferRequest{
		RequestID:    requestID,
		FromWalletID: testWalletA,
		ToWalletID:   testWalletB,
		Amount:       30,
		Currency:     "USD",
	})

	if err := env.client.Conn.Publish("wallet.transfer", payload); err != nil {
		t.Fatalf("publish transfer: %v", err)
	}

	event := env.waitForEvent(t, "wallet.events.completed")
	if event.RequestID != requestID || event.Operation != "transfer" {
		t.Fatalf("unexpected event: %+v", event)
	}

	if balance := env.walletBalance(t, testWalletA); balance != 70 {
		t.Fatalf("wallet A balance = %v, want 70", balance)
	}
	if balance := env.walletBalance(t, testWalletB); balance != 70 {
		t.Fatalf("wallet B balance = %v, want 70", balance)
	}
}

func TestHandleTransfer_InsufficientFundsPublishesFailed(t *testing.T) {
	env := setupHandlerTestEnv(t)

	seedDeposit(t, env, "70000000-0000-0000-0000-000000000004", testWalletA, 20)
	seedDeposit(t, env, "70000000-0000-0000-0000-000000000005", testWalletB, 10)

	payload, _ := json.Marshal(TransferRequest{
		RequestID:    "70000000-0000-0000-0000-000000000006",
		FromWalletID: testWalletA,
		ToWalletID:   testWalletB,
		Amount:       50,
		Currency:     "USD",
	})

	if err := env.client.Conn.Publish("wallet.transfer", payload); err != nil {
		t.Fatalf("publish transfer: %v", err)
	}

	event := env.waitForEvent(t, "wallet.events.failed")
	if event.Status != "failed" {
		t.Fatalf("event status = %q, want failed", event.Status)
	}

	if balance := env.walletBalance(t, testWalletA); balance != 20 {
		t.Fatalf("wallet A balance = %v, want 20", balance)
	}
	if balance := env.walletBalance(t, testWalletB); balance != 10 {
		t.Fatalf("wallet B balance = %v, want 10", balance)
	}
}

func TestHandleTransfer_DuplicateRequestIsIdempotent(t *testing.T) {
	env := setupHandlerTestEnv(t)

	seedDeposit(t, env, "70000000-0000-0000-0000-000000000007", testWalletA, 100)
	seedDeposit(t, env, "70000000-0000-0000-0000-000000000008", testWalletB, 50)

	const requestID = "70000000-0000-0000-0000-000000000009"
	payload, _ := json.Marshal(TransferRequest{
		RequestID:    requestID,
		FromWalletID: testWalletA,
		ToWalletID:   testWalletB,
		Amount:       20,
		Currency:     "USD",
	})

	if err := env.client.Conn.Publish("wallet.transfer", payload); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	env.waitForEvent(t, "wallet.events.completed")

	if err := env.client.Conn.Publish("wallet.transfer", payload); err != nil {
		t.Fatalf("duplicate publish: %v", err)
	}
	env.waitForEvent(t, "wallet.events.completed")

	if balance := env.walletBalance(t, testWalletA); balance != 80 {
		t.Fatalf("wallet A balance = %v, want 80", balance)
	}
	if balance := env.walletBalance(t, testWalletB); balance != 70 {
		t.Fatalf("wallet B balance = %v, want 70", balance)
	}
}
