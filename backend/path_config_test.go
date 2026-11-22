package backend

import (
	"testing"

	"github.com/openbao/openbao/sdk/v2/logical"
)

func TestConfigDefaults(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)

	resp := request(t, b, s, logical.ReadOperation, "config", nil)
	if resp.Data["socket"] != "" || resp.Data["timeout"] != int64(2) {
		t.Errorf("config = %v, want an empty socket and a timeout of 2", resp.Data)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)

	resp := request(t, b, s, logical.UpdateOperation, "config", map[string]any{
		"socket": "/run/ts.sock", "timeout": "5s",
	})
	if resp.IsError() {
		t.Fatalf("write config: %v", resp.Error())
	}

	resp = request(t, b, s, logical.ReadOperation, "config", nil)
	if resp.Data["socket"] != "/run/ts.sock" || resp.Data["timeout"] != int64(5) {
		t.Errorf("config = %v, want /run/ts.sock and 5", resp.Data)
	}
}

func TestConfigPartialUpdateKeepsOtherField(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)
	request(t, b, s, logical.UpdateOperation, "config", map[string]any{"socket": "/run/ts.sock"})
	request(t, b, s, logical.UpdateOperation, "config", map[string]any{"timeout": "7s"})

	resp := request(t, b, s, logical.ReadOperation, "config", nil)
	if resp.Data["socket"] != "/run/ts.sock" || resp.Data["timeout"] != int64(7) {
		t.Errorf("config = %v, want /run/ts.sock and 7", resp.Data)
	}
}

func TestConfigRejectsBadTimeout(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)

	for _, timeout := range []string{"0", "-1s"} {
		resp := request(
			t,
			b,
			s,
			logical.UpdateOperation,
			"config",
			map[string]any{"timeout": timeout},
		)
		if !resp.IsError() {
			t.Errorf("timeout %q was accepted", timeout)
		}
	}
}
