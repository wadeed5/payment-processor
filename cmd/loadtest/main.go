package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/fundingpips/wallet-service/internal/config"
	"github.com/fundingpips/wallet-service/internal/handler"
	walletnats "github.com/fundingpips/wallet-service/internal/nats"
	"github.com/fundingpips/wallet-service/internal/storage"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type depositReq struct {
	RequestID string  `json:"request_id"`
	WalletID  string  `json:"wallet_id"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
}

type withdrawReq struct {
	RequestID string  `json:"request_id"`
	WalletID  string  `json:"wallet_id"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
}

type transferReq struct {
	RequestID    string  `json:"request_id"`
	FromWalletID string  `json:"from_wallet_id"`
	ToWalletID   string  `json:"to_wallet_id"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
}

type balanceReq struct {
	WalletID string `json:"wallet_id"`
}

type balanceResp struct {
	WalletID string  `json:"wallet_id"`
	Balance  float64 `json:"balance"`
	Currency string  `json:"currency"`
}

type stats struct {
	deposits  int
	withdraws int
	transfers int
	balances  int
	failed    int
}

func main() {
	ops := flag.Int("ops", 200, "number of random operations after initial funding")
	initial := flag.Float64("initial", 10000, "initial balance per account")
	flag.Parse()

	cfg := config.Load()

	store, err := storage.NewStore(cfg.PGUrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres connect failed: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	nc, err := walletnats.Connect(cfg.NATSUrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nats connect failed: %v\n", err)
		os.Exit(1)
	}
	defer nc.Close()

	if err := subscribeHandlers(store, nc); err != nil {
		fmt.Fprintf(os.Stderr, "subscribe handlers: %v\n", err)
		os.Exit(1)
	}
	if err := nc.Conn.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "nats flush: %v\n", err)
		os.Exit(1)
	}

	completedSub, err := nc.Conn.SubscribeSync("wallet.events.completed")
	if err != nil {
		fmt.Fprintf(os.Stderr, "subscribe completed events: %v\n", err)
		os.Exit(1)
	}
	failedSub, err := nc.Conn.SubscribeSync("wallet.events.failed")
	if err != nil {
		fmt.Fprintf(os.Stderr, "subscribe failed events: %v\n", err)
		os.Exit(1)
	}

	wallets := []string{
		uuid.NewString(),
		uuid.NewString(),
		uuid.NewString(),
	}

	fmt.Println("=== wallet load test ===")
	fmt.Printf("NATS: %s\n", cfg.NATSUrl)
	fmt.Printf("Postgres: %s\n", cfg.PGUrl)
	fmt.Printf("Accounts: %d | Initial balance each: %.2f | Random ops: %d\n\n", len(wallets), *initial, *ops)

	if err := resetData(store); err != nil {
		fmt.Fprintf(os.Stderr, "reset data: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		fmt.Println("\n==> cleaning up test data...")
		if err := resetData(store); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup failed: %v\n", err)
			return
		}
		fmt.Println("    all wallets and transactions deleted")
	}()

	var s stats
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	fmt.Println("==> funding accounts")
	for i, walletID := range wallets {
		requestID := uuid.NewString()
		if err := publishDeposit(nc, completedSub, failedSub, requestID, walletID, *initial, &s); err != nil {
			fmt.Fprintf(os.Stderr, "initial deposit account %d failed: %v\n", i+1, err)
			os.Exit(1)
		}
		fmt.Printf("    account %d: %s -> %.2f USD\n", i+1, walletID, *initial)
	}

	fmt.Printf("\n==> running %d random operations\n", *ops)
	for i := 0; i < *ops; i++ {
		op := rng.Intn(4)
		switch op {
		case 0:
			wallet := wallets[rng.Intn(len(wallets))]
			amount := randomAmount(rng, 1, 100)
			requestID := uuid.NewString()
			if err := publishWithdraw(nc, completedSub, failedSub, requestID, wallet, amount, &s); err != nil {
				fmt.Fprintf(os.Stderr, "op %d withdraw error: %v\n", i+1, err)
			}
		case 1:
			from := wallets[rng.Intn(len(wallets))]
			to := wallets[rng.Intn(len(wallets))]
			for to == from {
				to = wallets[rng.Intn(len(wallets))]
			}
			amount := randomAmount(rng, 1, 150)
			requestID := uuid.NewString()
			if err := publishTransfer(nc, completedSub, failedSub, requestID, from, to, amount, &s); err != nil {
				fmt.Fprintf(os.Stderr, "op %d transfer error: %v\n", i+1, err)
			}
		case 2:
			wallet := wallets[rng.Intn(len(wallets))]
			if _, err := requestBalance(nc, wallet); err != nil {
				fmt.Fprintf(os.Stderr, "op %d balance error: %v\n", i+1, err)
			} else {
				s.balances++
			}
		case 3:
			wallet := wallets[rng.Intn(len(wallets))]
			amount := randomAmount(rng, 10, 50)
			requestID := uuid.NewString()
			if err := publishDeposit(nc, completedSub, failedSub, requestID, wallet, amount, &s); err != nil {
				fmt.Fprintf(os.Stderr, "op %d top-up deposit error: %v\n", i+1, err)
			}
		}

		if (i+1)%50 == 0 {
			fmt.Printf("    completed %d/%d operations\n", i+1, *ops)
		}
	}

	fmt.Printf("\n==> operation summary\n")
	fmt.Printf("    deposits : %d\n", s.deposits)
	fmt.Printf("    withdraws: %d (includes expected insufficient-fund failures)\n", s.withdraws)
	fmt.Printf("    transfers: %d (includes expected insufficient-fund failures)\n", s.transfers)
	fmt.Printf("    balances : %d\n", s.balances)
	fmt.Printf("    failed   : %d\n", s.failed)

	fmt.Println("\n==> final account report")
	discrepancies := 0
	for i, walletID := range wallets {
		walletBalance, currency, err := store.GetWalletBalance(walletID)
		if err != nil {
			fmt.Printf("    account %d: %s -> ERROR reading wallet: %v\n", i+1, walletID, err)
			discrepancies++
			continue
		}

		ledgerBalance, err := store.SumBalanceFromTransactions(walletID)
		if err != nil {
			fmt.Printf("    account %d: %s -> ERROR reading ledger: %v\n", i+1, walletID, err)
			discrepancies++
			continue
		}

		natsBalance, natsErr := requestBalance(nc, walletID)
		if natsErr != nil {
			fmt.Printf("    account %d: %s -> ERROR nats balance: %v\n", i+1, walletID, natsErr)
		}

		match := walletBalance == ledgerBalance
		status := "OK"
		if !match {
			status = "DISCREPANCY"
			discrepancies++
		}

		fmt.Printf("    account %d: %s\n", i+1, walletID)
		fmt.Printf("      wallet table : %.4f %s\n", walletBalance, currency)
		fmt.Printf("      ledger sum   : %.4f\n", ledgerBalance)
		if natsErr == nil {
			fmt.Printf("      nats balance : %.4f %s\n", natsBalance.Balance, natsBalance.Currency)
			if natsBalance.Balance != walletBalance {
				fmt.Printf("      nats vs wallet: DISCREPANCY (diff %.4f)\n", natsBalance.Balance-walletBalance)
				discrepancies++
			}
		}
		fmt.Printf("      wallet vs ledger: %s", status)
		if !match {
			fmt.Printf(" (diff %.4f)", walletBalance-ledgerBalance)
		}
		fmt.Println()
	}

	fmt.Println("\n==> result")
	if discrepancies == 0 {
		fmt.Println("    no discrepancies detected")
		return
	}
	fmt.Printf("    found %d discrepancy issue(s)\n", discrepancies)
	os.Exit(1)
}

func subscribeHandlers(store *storage.Store, nc *walletnats.Client) error {
	if _, err := nc.Conn.Subscribe("wallet.deposit", handler.HandleDeposit(store, nc)); err != nil {
		return err
	}
	if _, err := nc.Conn.Subscribe("wallet.withdraw", handler.HandleWithdraw(store, nc)); err != nil {
		return err
	}
	if _, err := nc.Conn.Subscribe("wallet.transfer", handler.HandleTransfer(store, nc)); err != nil {
		return err
	}
	if _, err := nc.Conn.Subscribe("wallet.balance", handler.HandleBalance(store)); err != nil {
		return err
	}
	return nil
}

func resetData(store *storage.Store) error {
	if _, err := store.DB.Exec(`TRUNCATE transactions RESTART IDENTITY`); err != nil {
		return err
	}
	if _, err := store.DB.Exec(`TRUNCATE wallets`); err != nil {
		return err
	}
	return nil
}

func publishDeposit(nc *walletnats.Client, completedSub, failedSub *nats.Subscription, requestID, walletID string, amount float64, s *stats) error {
	payload, err := json.Marshal(depositReq{
		RequestID: requestID,
		WalletID:  walletID,
		Amount:    amount,
		Currency:  "USD",
	})
	if err != nil {
		return err
	}
	if err := nc.Conn.Publish("wallet.deposit", payload); err != nil {
		return err
	}
	if err := waitOutcome(completedSub, failedSub, requestID); err != nil {
		s.failed++
		return err
	}
	s.deposits++
	return nil
}

func publishWithdraw(nc *walletnats.Client, completedSub, failedSub *nats.Subscription, requestID, walletID string, amount float64, s *stats) error {
	payload, err := json.Marshal(withdrawReq{
		RequestID: requestID,
		WalletID:  walletID,
		Amount:    amount,
		Currency:  "USD",
	})
	if err != nil {
		return err
	}
	if err := nc.Conn.Publish("wallet.withdraw", payload); err != nil {
		return err
	}
	if err := waitOutcome(completedSub, failedSub, requestID); err != nil {
		s.failed++
		return nil
	}
	s.withdraws++
	return nil
}

func publishTransfer(nc *walletnats.Client, completedSub, failedSub *nats.Subscription, requestID, fromWallet, toWallet string, amount float64, s *stats) error {
	payload, err := json.Marshal(transferReq{
		RequestID:    requestID,
		FromWalletID: fromWallet,
		ToWalletID:   toWallet,
		Amount:       amount,
		Currency:     "USD",
	})
	if err != nil {
		return err
	}
	if err := nc.Conn.Publish("wallet.transfer", payload); err != nil {
		return err
	}
	if err := waitOutcome(completedSub, failedSub, requestID); err != nil {
		s.failed++
		return nil
	}
	s.transfers++
	return nil
}

func requestBalance(nc *walletnats.Client, walletID string) (balanceResp, error) {
	payload, err := json.Marshal(balanceReq{WalletID: walletID})
	if err != nil {
		return balanceResp{}, err
	}
	msg, err := nc.Conn.Request("wallet.balance", payload, 3*time.Second)
	if err != nil {
		return balanceResp{}, err
	}
	var resp balanceResp
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return balanceResp{}, err
	}
	return resp, nil
}

func waitOutcome(completedSub, failedSub *nats.Subscription, requestID string) error {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if msg, err := completedSub.NextMsg(100 * time.Millisecond); err == nil {
			var event struct {
				RequestID string `json:"request_id"`
			}
			if json.Unmarshal(msg.Data, &event) == nil && event.RequestID == requestID {
				return nil
			}
		}
		if msg, err := failedSub.NextMsg(100 * time.Millisecond); err == nil {
			var event struct {
				RequestID string `json:"request_id"`
			}
			if json.Unmarshal(msg.Data, &event) == nil && event.RequestID == requestID {
				return fmt.Errorf("operation failed")
			}
		}
	}
	return fmt.Errorf("timeout waiting for outcome event")
}

func randomAmount(rng *rand.Rand, min, max int) float64 {
	return float64(min + rng.Intn(max-min+1))
}
