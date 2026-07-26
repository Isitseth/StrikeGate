package gate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigRetainsRouteSecretWithoutSerializingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte(`
config:
  bind: 127.0.0.1:25565
  forwarding:
    mode: none
  routes:
    - hostnames: [play.example.com]
      backend: 127.0.0.1:25566
      forwarding:
        mode: velocity
        secret: customer-secret
`), 0o600))
	v := viper.New()
	v.SetConfigFile(path)
	cfg, err := LoadConfig(v)
	require.NoError(t, err)
	require.Len(t, cfg.Config.Routes, 1)
	require.Equal(t, "customer-secret", string(cfg.Config.Routes[0].Forwarding.Secret))
}
