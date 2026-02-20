package discovery

import (
	"fmt"
	"net"
	"os"
	"strings"
	"log/slog"

	"github.com/hashicorp/mdns"
)

// Metadata holds the TXT record fields for the service.
type Metadata struct {
	Role        string // e.g., "gateway"
	Transport   string // e.g., "gateway"
	GatewayPort string // port number as string
	LanHost     string // e.g., "my-mac.local"
	DisplayName string // e.g., "My Mac"
	RemoteID    string // e.g., device ID of the gateway
}

// Config holds configuration for the mDNS advertiser.
type Config struct {
	InstanceName string // Name of the service instance
	Port         int    // Port where the service is running
	LanHost      string // Optional: Hostname to advertise
	Meta         Metadata
}

// Advertiser manages the mDNS service registration.
type Advertiser struct {
	servers []*mdns.Server
	cfg     Config
}

// NewAdvertiser creates a new advertiser with the given config.
func NewAdvertiser(cfg Config) (*Advertiser, error) {
	if cfg.InstanceName == "" {
		return nil, fmt.Errorf("instance name is required")
	}
	if cfg.Port <= 0 {
		return nil, fmt.Errorf("port must be > 0")
	}

	return &Advertiser{
		cfg: cfg,
	}, nil
}

// Start begins advertising the service.
// It returns immediately, running the server in a goroutine (managed by mdns lib).
func (a *Advertiser) Start() error {
	// Build TXT records
	txt := []string{
		fmt.Sprintf("role=%s", a.cfg.Meta.Role),
		fmt.Sprintf("transport=%s", a.cfg.Meta.Transport),
		fmt.Sprintf("gatewayPort=%s", a.cfg.Meta.GatewayPort),
		fmt.Sprintf("lanHost=%s", a.cfg.Meta.LanHost),
		fmt.Sprintf("displayName=%s", a.cfg.Meta.DisplayName),
	}
	if a.cfg.Meta.RemoteID != "" {
		txt = append(txt, fmt.Sprintf("remoteId=%s", a.cfg.Meta.RemoteID))
	}

	// Create service definition
	// Service Type: _openclaw-gw._tcp
	service, err := mdns.NewMDNSService(
		a.cfg.InstanceName,
		"_openclaw-gw._tcp",
		"",
		"",
		a.cfg.Port,
		nil, // IPs (nil = all interfaces)
		txt,
	)
	if err != nil {
		return fmt.Errorf("create mdns service: %w", err)
	}

	// Create and start servers on multicast-capable interfaces.
	// mdns.NewServer triggers advertisement immediately.
	ifaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("list interfaces: %w", err)
	}

	var servers []*mdns.Server
	ifaceFilter := strings.TrimSpace(os.Getenv("GOCLAW_MDNS_IFACE"))
	for _, iface := range ifaces {
		iface := iface
		if ifaceFilter != "" && iface.Name != ifaceFilter {
			continue
		}
		if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagMulticast) == 0 {
			continue
		}

		server, err := mdns.NewServer(&mdns.Config{
			Zone:             service,
			Iface:            &iface,
			LogEmptyResponses: true,
		})
		if err != nil {
			slog.Warn("mdns interface bind failed", "iface", iface.Name, "error", err)
			continue
		}
		slog.Info("mdns interface bound", "iface", iface.Name)
		servers = append(servers, server)
	}

	// Fallback to default interface if none succeeded and no explicit filter.
	if len(servers) == 0 && ifaceFilter == "" {
		server, err := mdns.NewServer(&mdns.Config{
			Zone:             service,
			LogEmptyResponses: true,
		})
		if err != nil {
			return fmt.Errorf("start mdns server: %w", err)
		}
		servers = append(servers, server)
	}
	if len(servers) == 0 {
		return fmt.Errorf("no mdns interfaces bound (filter=%q)", ifaceFilter)
	}

	a.servers = servers
	return nil
}

// Stop shuts down the mDNS advertisement.
func (a *Advertiser) Stop() error {
	var firstErr error
	for _, server := range a.servers {
		if server == nil {
			continue
		}
		if err := server.Shutdown(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
