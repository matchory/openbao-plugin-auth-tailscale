package backend

import (
	"context"
	"fmt"
	"time"

	"github.com/openbao/openbao/sdk/v2/framework"
	"github.com/openbao/openbao/sdk/v2/logical"
)

const (
	configKey      = "config"
	defaultTimeout = 2 * time.Second
)

type config struct {
	// Socket is the path of tailscaled's socket. Empty means the
	// platform's default.
	Socket string `json:"socket"`
	// Timeout bounds one question to tailscaled, and with it a login.
	Timeout time.Duration `json:"timeout"`
}

func (b *backend) configPaths() []*framework.Path {
	return []*framework.Path{{
		Pattern: "config$",
		Fields: map[string]*framework.FieldSchema{
			"socket": {
				Type:        framework.TypeString,
				Description: "Path of the local tailscaled's socket. Empty uses the platform's default.",
			},
			"timeout": {
				Type:        framework.TypeDurationSecond,
				Description: "How long to wait for tailscaled before refusing a login. Default 2s.",
			},
		},
		Operations: map[logical.Operation]framework.OperationHandler{
			logical.ReadOperation: &framework.PathOperation{
				Callback: b.readConfig,
				Summary:  "Read the method's configuration.",
			},
			logical.UpdateOperation: &framework.PathOperation{
				Callback: b.writeConfig,
				Summary:  "Configure how the method reaches tailscaled.",
			},
		},
		HelpSynopsis: "Configure how the method reaches the local tailscaled.",
	}}
}

// config returns the stored configuration, or the defaults when none is
// stored.
func (b *backend) config(ctx context.Context, s logical.Storage) (*config, error) {
	cfg := &config{Timeout: defaultTimeout}

	entry, err := s.Get(ctx, configKey)
	if err != nil {
		return nil, fmt.Errorf("read the configuration: %w", err)
	}

	if entry == nil {
		return cfg, nil
	}

	if err := entry.DecodeJSON(cfg); err != nil {
		return nil, fmt.Errorf("decode the configuration: %w", err)
	}

	return cfg, nil
}

func (b *backend) readConfig(ctx context.Context, req *logical.Request, _ *framework.FieldData) (
	*logical.Response, error,
) {
	cfg, err := b.config(ctx, req.Storage)
	if err != nil {
		return nil, err
	}

	return &logical.Response{Data: map[string]any{
		"socket":  cfg.Socket,
		"timeout": int64(cfg.Timeout.Seconds()),
	}}, nil
}

func (b *backend) writeConfig(ctx context.Context, req *logical.Request, d *framework.FieldData) (
	*logical.Response, error,
) {
	cfg, err := b.config(ctx, req.Storage)
	if err != nil {
		return nil, err
	}

	// The framework has already converted each field to its schema's type.
	if socket, ok := d.GetOk("socket"); ok {
		cfg.Socket, _ = socket.(string)
	}

	if timeout, ok := d.GetOk("timeout"); ok {
		seconds, _ := timeout.(int)
		cfg.Timeout = time.Duration(seconds) * time.Second
	}

	if cfg.Timeout <= 0 {
		return logical.ErrorResponse("timeout must be greater than zero"), nil
	}

	entry, err := logical.StorageEntryJSON(configKey, cfg)
	if err != nil {
		return nil, fmt.Errorf("encode the configuration: %w", err)
	}

	if err := req.Storage.Put(ctx, entry); err != nil {
		return nil, fmt.Errorf("store the configuration: %w", err)
	}

	return nil, nil //nolint:nilnil // an empty response is how a write reports success
}
