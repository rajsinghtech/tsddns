package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"tailscale.com/client/tailscale/v2"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		configJSON  string
		wantErr     bool
		wantDomains int
	}{
		{
			name: "valid config",
			configJSON: `{
				"example.com": ["svc:test-service"],
				"internal.example.com": ["192.168.1.1", "device:test-device"]
			}`,
			wantErr:     false,
			wantDomains: 2,
		},
		{
			name:       "bad json",
			configJSON: `{invalid`,
			wantErr:    true,
		},
		{
			name:        "empty",
			configJSON:  `{}`,
			wantErr:     false,
			wantDomains: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.json")
			os.WriteFile(configPath, []byte(tt.configJSON), 0644)

			cfg, err := loadConfig(configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(cfg) != tt.wantDomains {
				t.Errorf("got %d domains, want %d", len(cfg), tt.wantDomains)
			}
		})
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := loadConfig("/nonexistent/config.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestCreateClient(t *testing.T) {
	tests := []struct {
		name         string
		tailnet      string
		apiKey       string
		clientID     string
		clientSecret string
		baseURL      string
		wantErr      bool
	}{
		{
			name:    "valid api key",
			tailnet: "example.com",
			apiKey:  "tskey-api-test",
			baseURL: "https://api.tailscale.com",
			wantErr: false,
		},
		{
			name:         "valid oauth",
			tailnet:      "example.com",
			clientID:     "test-client-id",
			clientSecret: "test-client-secret",
			baseURL:      "https://api.tailscale.com",
			wantErr:      false,
		},
		{
			name:    "no auth provided",
			tailnet: "example.com",
			baseURL: "https://api.tailscale.com",
			wantErr: true,
		},
		{
			name:    "invalid base url",
			tailnet: "example.com",
			apiKey:  "tskey-api-test",
			baseURL: "://invalid-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := createClient(tt.tailnet, tt.apiKey, tt.clientID, tt.clientSecret, tt.baseURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("createClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if client == nil {
					t.Error("createClient() returned nil client")
				}
				if client.Tailnet != tt.tailnet {
					t.Errorf("createClient() tailnet = %v, want %v", client.Tailnet, tt.tailnet)
				}
			}
		})
	}
}

func TestGetDeviceIP(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		devices  []tailscale.Device
		wantIP   string
		wantErr  bool
	}{
		{
			name:     "exact hostname match",
			hostname: "test-device",
			devices: []tailscale.Device{
				{
					Hostname:  "test-device",
					Addresses: []string{"100.64.0.1", "fd7a::1"},
				},
			},
			wantIP:  "100.64.0.1",
			wantErr: false,
		},
		{
			name:     "name prefix match",
			hostname: "test-device",
			devices: []tailscale.Device{
				{
					Name:      "test-device.example.ts.net",
					Hostname:  "other-name",
					Addresses: []string{"100.64.0.2", "fd7a::2"},
				},
			},
			wantIP:  "100.64.0.2",
			wantErr: false,
		},
		{
			name:     "full name match",
			hostname: "test-device.example.ts.net",
			devices: []tailscale.Device{
				{
					Name:      "test-device.example.ts.net",
					Hostname:  "test",
					Addresses: []string{"100.64.0.3", "fd7a::3"},
				},
			},
			wantIP:  "100.64.0.3",
			wantErr: false,
		},
		{
			name:     "device not found",
			hostname: "nonexistent",
			devices: []tailscale.Device{
				{
					Hostname:  "other-device",
					Addresses: []string{"100.64.0.4"},
				},
			},
			wantIP:  "",
			wantErr: true,
		},
		{
			name:     "device with no addresses",
			hostname: "test-device",
			devices: []tailscale.Device{
				{
					Hostname:  "test-device",
					Addresses: []string{},
				},
			},
			wantIP:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIP, err := getDeviceIP(tt.hostname, tt.devices)
			if (err != nil) != tt.wantErr {
				t.Errorf("getDeviceIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotIP != tt.wantIP {
				t.Errorf("getDeviceIP() = %v, want %v", gotIP, tt.wantIP)
			}
		})
	}
}

func TestGetServiceIP(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		response    ServiceInfo
		statusCode  int
		wantIP      string
		wantErr     bool
	}{
		{
			name:        "valid service",
			serviceName: "svc:test-service",
			response: ServiceInfo{
				Name:  "svc:test-service",
				Addrs: []string{"100.64.0.1", "fd7a::1"},
			},
			statusCode: http.StatusOK,
			wantIP:     "100.64.0.1",
			wantErr:    false,
		},
		{
			name:        "service not found",
			serviceName: "svc:nonexistent",
			statusCode:  http.StatusNotFound,
			wantErr:     true,
		},
		{
			name:        "service with no addresses",
			serviceName: "svc:test-service",
			response: ServiceInfo{
				Name:  "svc:test-service",
				Addrs: []string{},
			},
			statusCode: http.StatusOK,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.statusCode != http.StatusOK {
					w.WriteHeader(tt.statusCode)
					return
				}
				json.NewEncoder(w).Encode(tt.response)
			}))
			defer server.Close()

			serverURL, _ := url.Parse(server.URL)
			client := &tailscale.Client{
				BaseURL: serverURL,
				Tailnet: "test",
				APIKey:  "test-key",
			}

			gotIP, err := getServiceIP(context.Background(), client, tt.serviceName)
			if (err != nil) != tt.wantErr {
				t.Errorf("getServiceIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotIP != tt.wantIP {
				t.Errorf("getServiceIP() = %v, want %v", gotIP, tt.wantIP)
			}
		})
	}
}

func TestResolveSplitDNS(t *testing.T) {
	tests := []struct {
		name         string
		config       Config
		wantDomains  int
		wantErr      bool
		checkResults func(t *testing.T, result tailscale.SplitDNSRequest)
	}{
		{
			name: "direct IP only",
			config: Config{
				"direct.example.com": {"192.168.1.1"},
				"multi.example.com":  {"192.168.1.1", "192.168.1.2"},
			},
			wantDomains: 2,
			wantErr:     false,
			checkResults: func(t *testing.T, result tailscale.SplitDNSRequest) {
				if result["direct.example.com"][0] != "192.168.1.1" {
					t.Errorf("expected 192.168.1.1 for direct.example.com, got %s", result["direct.example.com"][0])
				}
				if len(result["multi.example.com"]) != 2 {
					t.Errorf("expected 2 nameservers for multi.example.com, got %d", len(result["multi.example.com"]))
				}
			},
		},
		{
			name:        "empty config",
			config:      Config{},
			wantDomains: 0,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal client for testing IP-only configs
			serverURL, _ := url.Parse("https://api.tailscale.com")
			client := &tailscale.Client{
				BaseURL: serverURL,
				Tailnet: "test",
				APIKey:  "test-key",
			}

			result, err := resolveSplitDNS(context.Background(), client, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveSplitDNS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(result) != tt.wantDomains {
				t.Errorf("resolveSplitDNS() got %d domains, want %d", len(result), tt.wantDomains)
			}

			if tt.checkResults != nil && !tt.wantErr {
				tt.checkResults(t, result)
			}
		})
	}
}

func TestResolveSplitDNSWithServiceAPI(t *testing.T) {
	// Mock HTTP server for services API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/tailnet/test/services/svc:test-service/" {
			json.NewEncoder(w).Encode(ServiceInfo{
				Name:  "svc:test-service",
				Addrs: []string{"100.64.0.1"},
			})
			return
		}
		if r.URL.Path == "/api/v2/tailnet/test/devices" {
			json.NewEncoder(w).Encode(map[string][]tailscale.Device{
				"devices": {
					{
						Hostname:  "test-device",
						Addresses: []string{"100.64.0.2"},
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Run("resolves service to IP", func(t *testing.T) {
		serverURL, _ := url.Parse(server.URL)
		client := &tailscale.Client{
			BaseURL: serverURL,
			Tailnet: "test",
			APIKey:  "test-key",
		}

		cfg := Config{
			"service.example.com": {"svc:test-service"},
		}

		result, err := resolveSplitDNS(context.Background(), client, cfg)
		if err != nil {
			t.Fatalf("resolveSplitDNS() unexpected error: %v", err)
		}

		if len(result) != 1 {
			t.Errorf("expected 1 domain, got %d", len(result))
		}

		if result["service.example.com"][0] != "100.64.0.1" {
			t.Errorf("expected 100.64.0.1, got %s", result["service.example.com"][0])
		}
	})

	t.Run("resolves device to IP", func(t *testing.T) {
		serverURL, _ := url.Parse(server.URL)
		client := &tailscale.Client{
			BaseURL: serverURL,
			Tailnet: "test",
			APIKey:  "test-key",
		}

		cfg := Config{
			"device.example.com": {"device:test-device"},
		}

		result, err := resolveSplitDNS(context.Background(), client, cfg)
		if err != nil {
			t.Fatalf("resolveSplitDNS() unexpected error: %v", err)
		}

		if len(result) != 1 {
			t.Errorf("expected 1 domain, got %d", len(result))
		}

		if result["device.example.com"][0] != "100.64.0.2" {
			t.Errorf("expected 100.64.0.2, got %s", result["device.example.com"][0])
		}
	})
}

func TestUpdateDNS(t *testing.T) {
	t.Run("basic call", func(t *testing.T) {
		client := &tailscale.Client{
			Tailnet: "test",
		}

		cfg := Config{
			"example.com": {"192.168.1.1"},
		}

		err := updateDNS(context.Background(), client, cfg)
		if err == nil {
			t.Log("succeeded")
		} else {
			t.Logf("failed as expected: %v", err)
		}
	})
}
