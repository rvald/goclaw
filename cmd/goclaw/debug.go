package main

import (
	"fmt"
	"net"
	"os"

	"github.com/hashicorp/mdns"
	"github.com/spf13/cobra"
)

var debugDiscoveryCmd = &cobra.Command{
	Use:   "discovery",
	Short: "Debug Bonjour/mDNS discovery",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. List interfaces
		ifaces, err := net.Interfaces()
		if err != nil {
			return err
		}
		fmt.Println("Network Interfaces:")
		for _, iface := range ifaces {
			addrs, _ := iface.Addrs()
			fmt.Printf("- %s (Flags: %v)\n", iface.Name, iface.Flags)
			for _, addr := range addrs {
				fmt.Printf("  - %s\n", addr.String())
			}
		}
		fmt.Println()

		// 2. Start advertising
		fmt.Println("Starting mDNS advertisement on port 18789...")
		fmt.Println("Service: _openclaw-gw._tcp")
		
		host, _ := os.Hostname()
		info := []string{"role=gateway", "debug=true"}
		
		service, err := mdns.NewMDNSService(
			"OpenClaw Debug",
			"_openclaw-gw._tcp",
			"",
			"",
			18789,
			nil,
			info,
		)
		if err != nil {
			return err
		}

		server, err := mdns.NewServer(&mdns.Config{Zone: service})
		if err != nil {
			return fmt.Errorf("failed to start server: %w", err)
		}
		defer server.Shutdown()

		fmt.Printf("Advertising as 'OpenClaw Debug' on %s. Press Ctrl+C to stop.\n", host)
		
		// Keep alive
		select {}
	},
}

func init() {
	rootCmd.AddCommand(debugCmd)
	debugCmd.AddCommand(debugDiscoveryCmd)
}

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug utilities",
}
