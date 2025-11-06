module github.com/rajsingh/tsddns

go 1.24.0

require (
	golang.org/x/oauth2 v0.30.0
	tailscale.com/client/tailscale/v2 v2.0.0
)

require github.com/tailscale/hujson v0.0.0-20220506213045-af5ed07155e5 // indirect

replace tailscale.com/client/tailscale/v2 => ../tailscale-client-go-v2
