package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2/clientcredentials"
	tailscale "github.com/tailscale/tailscale-client-go/v2"
)

type Config map[string][]string

type ServiceInfo struct {
	Name  string   `json:"name"`
	Addrs []string `json:"addrs"`
}

func main() {
	configPath := flag.String("config", "/config.json", "Path to config.json")
	tailnet := flag.String("tailnet", "-", "Tailscale tailnet name")
	apiKey := flag.String("api-key", os.Getenv("TAILSCALE_API_KEY"), "Tailscale API key")
	clientID := flag.String("client-id", os.Getenv("TAILSCALE_CLIENT_ID"), "OAuth client ID")
	clientSecret := flag.String("client-secret", os.Getenv("TAILSCALE_CLIENT_SECRET"), "OAuth client secret")
	baseURL := flag.String("base-url", "https://api.tailscale.com", "API base URL")
	interval := flag.Duration("interval", 0, "Run continuously (e.g., 5m, 1h)")

	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	client, err := createClient(*tailnet, *apiKey, *clientID, *clientSecret, *baseURL)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()

	if *interval > 0 {
		log.Printf("Running in daemon mode with interval: %v", *interval)
		ticker := time.NewTicker(*interval)
		defer ticker.Stop()

		runUpdate := func() {
			if err := updateDNS(ctx, client, cfg); err != nil {
				log.Printf("Error updating DNS: %v", err)
			}
		}

		runUpdate()
		for range ticker.C {
			runUpdate()
		}
	} else {
		if err := updateDNS(ctx, client, cfg); err != nil {
			log.Fatalf("Failed to update DNS: %v", err)
		}
	}
}

func updateDNS(ctx context.Context, client *tailscale.Client, cfg Config) error {
	splitDNS, err := resolveSplitDNS(ctx, client, cfg)
	if err != nil {
		return fmt.Errorf("resolving services: %w", err)
	}

	log.Printf("Updating split DNS configuration with %d domains...", len(splitDNS))
	for domain, nameservers := range splitDNS {
		log.Printf("  %s -> %v", domain, nameservers)
	}

	if err := client.DNS().SetSplitDNS(ctx, splitDNS); err != nil {
		return fmt.Errorf("updating split DNS: %w", err)
	}

	log.Println("Successfully updated split DNS configuration")
	return nil
}

func createClient(tailnet, apiKey, clientID, clientSecret, baseURL string) (*tailscale.Client, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	client := &tailscale.Client{
		Tailnet: tailnet,
		BaseURL: parsedURL,
	}

	if clientID != "" && clientSecret != "" {
		log.Println("Using OAuth client credentials authentication")
		oauthConfig := clientcredentials.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TokenURL:     baseURL + "/api/v2/oauth/token",
		}
		client.HTTP = oauthConfig.Client(context.Background())
	} else if apiKey != "" {
		log.Println("Using API key authentication")
		client.APIKey = apiKey
	} else {
		return nil, fmt.Errorf("need either api key or oauth creds")
	}

	return client, nil
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config JSON: %w", err)
	}

	return cfg, nil
}

func resolveSplitDNS(ctx context.Context, client *tailscale.Client, cfg Config) (tailscale.SplitDNSRequest, error) {
	splitDNS := make(tailscale.SplitDNSRequest)

	// only fetch devices list if we actually need it
	var devices []tailscale.Device
	needsDevices := false
	for _, nameservers := range cfg {
		for _, ns := range nameservers {
			if strings.HasPrefix(ns, "device:") {
				needsDevices = true
				break
			}
		}
		if needsDevices {
			break
		}
	}

	if needsDevices {
		devs, err := client.Devices().List(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing devices: %w", err)
		}
		devices = devs
	}

	for domain, nameservers := range cfg {
		var resolved []string
		for _, ns := range nameservers {
			if strings.HasPrefix(ns, "svc:") {
				log.Printf("Resolving service %s for domain %s...", ns, domain)
				ip, err := getServiceIP(ctx, client, ns)
				if err != nil {
					return nil, fmt.Errorf("resolving service %s: %w", ns, err)
				}
				log.Printf("  Resolved %s to %s", ns, ip)
				resolved = append(resolved, ip)
			} else if strings.HasPrefix(ns, "device:") {
				deviceName := strings.TrimPrefix(ns, "device:")
				log.Printf("Resolving device %s for domain %s...", deviceName, domain)
				ip, err := getDeviceIP(deviceName, devices)
				if err != nil {
					return nil, fmt.Errorf("resolving device %s: %w", deviceName, err)
				}
				log.Printf("  Resolved device:%s to %s", deviceName, ip)
				resolved = append(resolved, ip)
			} else {
				resolved = append(resolved, ns)
			}
		}
		splitDNS[domain] = resolved
	}

	return splitDNS, nil
}

func getServiceIP(ctx context.Context, client *tailscale.Client, serviceName string) (string, error) {
	// TODO: use the official client once services API is added
	url := fmt.Sprintf("%s/api/v2/tailnet/%s/services/%s/", client.BaseURL.String(), client.Tailnet, serviceName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	var httpClient *http.Client
	if client.APIKey != "" {
		req.SetBasicAuth(client.APIKey, "")
		httpClient = &http.Client{}
	} else if client.HTTP != nil {
		httpClient = client.HTTP
	} else {
		return "", fmt.Errorf("no auth configured")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var svcInfo ServiceInfo
	if err := json.NewDecoder(resp.Body).Decode(&svcInfo); err != nil {
		return "", err
	}

	if len(svcInfo.Addrs) == 0 {
		return "", fmt.Errorf("service %s has no addresses", serviceName)
	}

	return svcInfo.Addrs[0], nil
}

func getDeviceIP(hostname string, devices []tailscale.Device) (string, error) {
	for _, device := range devices {
		if device.Hostname == hostname || device.Name == hostname || strings.HasPrefix(device.Name, hostname+".") {
			if len(device.Addresses) == 0 {
				return "", fmt.Errorf("device %s has no addresses", hostname)
			}
			return device.Addresses[0], nil
		}
	}
	return "", fmt.Errorf("device %s not found", hostname)
}
