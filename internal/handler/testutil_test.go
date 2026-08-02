package handler

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fundingpips/wallet-service/internal/nats"
	"github.com/fundingpips/wallet-service/internal/storage"
	natsgo "github.com/nats-io/nats.go"
	natsserver "github.com/nats-io/nats-server/v2/test"
)

const (
	testWalletA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	testWalletB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

type handlerTestEnv struct {
	store  *storage.Store
	client *nats.Client
	conn   *natsgo.Conn
}

func testPGURL() string {
	if url := os.Getenv("PG_URL"); url != "" {
		return url
	}
	return "postgres://walletuser:walletpass@localhost:5432/wallet?sslmode=disable"
}

func setupHandlerTestEnv(t *testing.T) *handlerTestEnv {
	t.Helper()

	ns := natsserver.RunRandClientPortServer()
	t.Cleanup(ns.Shutdown)

	store, err := storage.NewStore(testPGURL())
	if err != nil {
		t.Skipf("postgres unavailable (start docker-compose): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.DB.Exec(`TRUNCATE transactions RESTART IDENTITY`); err != nil {
		t.Fatalf("truncate transactions: %v", err)
	}
	if _, err := store.DB.Exec(`TRUNCATE wallets`); err != nil {
		t.Fatalf("truncate wallets: %v", err)
	}

	client, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("connect nats client: %v", err)
	}
	t.Cleanup(client.Close)

	conn, err := natsgo.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("connect nats conn: %v", err)
	}
	t.Cleanup(conn.Close)

	if _, err := client.Conn.Subscribe("wallet.deposit", HandleDeposit(store, client)); err != nil {
		t.Fatalf("subscribe deposit: %v", err)
	}
	if _, err := client.Conn.Subscribe("wallet.withdraw", HandleWithdraw(store, client)); err != nil {
		t.Fatalf("subscribe withdraw: %v", err)
	}
	if _, err := client.Conn.Subscribe("wallet.transfer", HandleTransfer(store, client)); err != nil {
		t.Fatalf("subscribe transfer: %v", err)
	}
	if _, err := client.Conn.Subscribe("wallet.balance", HandleBalance(store)); err != nil {
		t.Fatalf("subscribe balance: %v", err)
	}

	if err := conn.Flush(); err != nil {
		t.Fatalf("flush nats: %v", err)
	}

	return &handlerTestEnv{store: store, client: client, conn: conn}
}

func (env *handlerTestEnv) waitForEvent(t *testing.T, subject string) Event {
	t.Helper()

	sub, err := env.conn.SubscribeSync(subject)
	if err != nil {
		t.Fatalf("subscribe sync %s: %v", subject, err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	msg, err := sub.NextMsg(3 * time.Second)
	if err != nil {
		t.Fatalf("wait for event on %s: %v", subject, err)
	}

	var event Event
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	return event
}

func (env *handlerTestEnv) walletBalance(t *testing.T, walletID string) float64 {
	t.Helper()

	balance, _, err := env.store.GetWalletBalance(walletID)
	if err != nil {
		t.Fatalf("get wallet balance: %v", err)
	}
	return balance
}

type eventListeners struct {
	completed *natsgo.Subscription
	failed    *natsgo.Subscription
}

// prepareEventListeners subscribes to outcome subjects before publishing commands.
func (env *handlerTestEnv) prepareEventListeners(t *testing.T) *eventListeners {
	t.Helper()

	completed, err := env.conn.SubscribeSync("wallet.events.completed")
	if err != nil {
		t.Fatalf("subscribe completed events: %v", err)
	}
	failed, err := env.conn.SubscribeSync("wallet.events.failed")
	if err != nil {
		t.Fatalf("subscribe failed events: %v", err)
	}
	t.Cleanup(func() {
		_ = completed.Unsubscribe()
		_ = failed.Unsubscribe()
	})

	return &eventListeners{completed: completed, failed: failed}
}

func (l *eventListeners) expectCompleted(t *testing.T, requestID, operation string) Event {
	t.Helper()

	msg, err := l.completed.NextMsg(3 * time.Second)
	if err != nil {
		t.Fatalf("wait for completed event: %v", err)
	}

	var event Event
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal completed event: %v", err)
	}
	if event.RequestID != requestID {
		t.Fatalf("completed request_id = %q, want %q", event.RequestID, requestID)
	}
	if event.Operation != operation {
		t.Fatalf("completed operation = %q, want %q", event.Operation, operation)
	}
	if event.Status != "completed" {
		t.Fatalf("completed status = %q, want completed", event.Status)
	}
	if event.Timestamp == "" {
		t.Fatal("completed event missing timestamp")
	}
	return event
}

func (l *eventListeners) expectFailed(t *testing.T, requestID, operation, reasonContains string) Event {
	t.Helper()

	msg, err := l.failed.NextMsg(3 * time.Second)
	if err != nil {
		t.Fatalf("wait for failed event: %v", err)
	}

	var event Event
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal failed event: %v", err)
	}
	if event.RequestID != requestID {
		t.Fatalf("failed request_id = %q, want %q", event.RequestID, requestID)
	}
	if event.Operation != operation {
		t.Fatalf("failed operation = %q, want %q", event.Operation, operation)
	}
	if event.Status != "failed" {
		t.Fatalf("failed status = %q, want failed", event.Status)
	}
	if event.Reason == nil || *event.Reason == "" {
		t.Fatal("failed event missing reason")
	}
	if reasonContains != "" && !strings.Contains(*event.Reason, reasonContains) {
		t.Fatalf("failed reason = %q, want substring %q", *event.Reason, reasonContains)
	}
	return event
}

func (l *eventListeners) expectNoEvents(t *testing.T, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if msg, _ := l.completed.NextMsg(50 * time.Millisecond); msg != nil {
			t.Fatalf("unexpected completed event: %s", string(msg.Data))
		}
		if msg, _ := l.failed.NextMsg(50 * time.Millisecond); msg != nil {
			t.Fatalf("unexpected failed event: %s", string(msg.Data))
		}
	}
}

func (env *handlerTestEnv) requestBalance(t *testing.T, walletID string) BalanceResponse {
	t.Helper()

	payload, err := json.Marshal(BalanceRequest{WalletID: walletID})
	if err != nil {
		t.Fatalf("marshal balance request: %v", err)
	}
	respMsg, err := env.conn.Request("wallet.balance", payload, 3*time.Second)
	if err != nil {
		t.Fatalf("balance request: %v", err)
	}

	var resp BalanceResponse
	if err := json.Unmarshal(respMsg.Data, &resp); err != nil {
		t.Fatalf("unmarshal balance response: %v", err)
	}
	return resp
}

func seedDeposit(t *testing.T, env *handlerTestEnv, requestID, walletID string, amount float64) {
	t.Helper()

	events := env.prepareEventListeners(t)
	payload, err := json.Marshal(DepositRequest{
		RequestID: requestID,
		WalletID:  walletID,
		Amount:    amount,
		Currency:  "USD",
	})
	if err != nil {
		t.Fatalf("marshal deposit seed: %v", err)
	}
	if err := env.client.Conn.Publish("wallet.deposit", payload); err != nil {
		t.Fatalf("publish deposit seed: %v", err)
	}
	events.expectCompleted(t, requestID, "deposit")
}

func ledgerCount(t *testing.T, env *handlerTestEnv) int {
	t.Helper()

	var count int
	if err := env.store.DB.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&count); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	return count
}
