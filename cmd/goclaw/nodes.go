package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/rvald/goclaw/internal/pairing"
	"github.com/spf13/cobra"
)

var nodesCmd = &cobra.Command{
	Use:   "nodes",
	Short: "Manage paired devices",
	Long:  `Manage device pairing: list pending requests, approve or reject them.`,
}

var nodesPendingCmd = &cobra.Command{
	Use:   "pending",
	Short: "List pending pairing requests",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openPairingStore()
		if err != nil {
			return err
		}

		pending := store.ListPending()
		if len(pending) == 0 {
			fmt.Println("No pending requests.")
			return nil
		}

		fmt.Printf("%-36s  %-20s  %-15s  %s\n", "REQUEST ID", "DEVICE NAME", "IP", "AGE")
		now := time.Now().UnixMilli()
		for _, req := range pending {
			age := time.Duration((now - req.Timestamp) * int64(time.Millisecond)).Round(time.Second)
			fmt.Printf("%-36s  %-20s  %-15s  %s\n", req.RequestID, req.DisplayName, req.RemoteIP, age)
		}
		return nil
	},
}

var nodesApproveCmd = &cobra.Command{
	Use:   "approve [request-id]",
	Short: "Approve a pending pairing request",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openPairingStore()
		if err != nil {
			return err
		}
		svc := pairing.NewService(store)

		reqID := args[0]
		device, err := svc.Approve(reqID)
		if err != nil {
			return fmt.Errorf("approve failed: %w", err)
		}
		if device == nil {
			return fmt.Errorf("request not found: %s", reqID)
		}

		fmt.Printf("Approved request %s\n", reqID)
		fmt.Printf("Device paired: %s (%s)\n", device.DisplayName, device.DeviceID)
		return nil
	},
}

var nodesRejectCmd = &cobra.Command{
	Use:   "reject [request-id]",
	Short: "Reject a pending pairing request",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openPairingStore()
		if err != nil {
			return err
		}
		svc := pairing.NewService(store)

		reqID := args[0]
		removed, err := svc.Reject(reqID)
		if err != nil {
			return fmt.Errorf("reject failed: %w", err)
		}
		if removed == nil {
			return fmt.Errorf("request not found: %s", reqID)
		}

		fmt.Printf("Rejected request %s from %s\n", reqID, removed.DisplayName)
		return nil
	},
}

var nodesStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "List paired devices",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openPairingStore()
		if err != nil {
			return err
		}

		paired := store.ListPaired()
		if len(paired) == 0 {
			fmt.Println("No paired devices.")
			return nil
		}

		fmt.Printf("%-36s  %-20s  %-15s  %s\n", "DEVICE ID", "NAME", "PLATFORM", "APPROVED")
		for _, dev := range paired {
			approved := time.UnixMilli(dev.ApprovedAtMs).Format(time.DateTime)
			fmt.Printf("%-36s  %-20s  %-15s  %s\n", dev.DeviceID, dev.DisplayName, dev.Platform, approved)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(nodesCmd)
	nodesCmd.AddCommand(nodesPendingCmd)
	nodesCmd.AddCommand(nodesApproveCmd)
	nodesCmd.AddCommand(nodesRejectCmd)
	nodesCmd.AddCommand(nodesStatusCmd)
}

func openPairingStore() (*pairing.Store, error) {
	// Root flags are parsed before Run, so cfgStateDir is populated
	path := filepath.Join(cfgStateDir, "pairing")
	store, err := pairing.NewStore(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open pairing store at %s: %w", path, err)
	}
	return store, nil
}
