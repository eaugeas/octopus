package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// serverConfig is a Binder used for testing that exposes a couple
// of flags
type serverConfig struct {
	Host string
	Port int

	bindErr      error
	configureErr error
}

func (c *serverConfig) Bind(v *viper.Viper, cmd *cobra.Command) error {
	cmd.PersistentFlags().String("server.host", "localhost", "host to listen on")
	cmd.PersistentFlags().Int("server.port", 8080, "port to listen on")
	return c.bindErr
}

func (c *serverConfig) Configure(v *viper.Viper) error {
	if c.configureErr != nil {
		return c.configureErr
	}

	c.Host = v.GetString("server.host")
	c.Port = v.GetInt("server.port")
	return nil
}

type appConfig struct {
	server *serverConfig
}

func (c *appConfig) Use() string       { return "app" }
func (c *appConfig) EnvPrefix() string { return "app" }
func (c *appConfig) Binders() []Binder { return []Binder{c.server} }

func newAppConfig() *appConfig {
	return &appConfig{server: &serverConfig{}}
}

func writeFile(t *testing.T, name, content string) string {
	p := filepath.Join(t.TempDir(), name)
	assert.Nil(t, os.WriteFile(p, []byte(content), 0600))
	return p
}

func TestParseDefaults(t *testing.T) {
	config := newAppConfig()
	parser, err := Generate("app", config)
	assert.Nil(t, err)

	assert.Nil(t, parser.ParseArgs([]string{"app"}))
	assert.Equal(t, "localhost", config.server.Host)
	assert.Equal(t, 8080, config.server.Port)
	assert.Equal(t, "", parser.file.Path)
}

func TestParseFlags(t *testing.T) {
	config := newAppConfig()
	parser, err := Generate("app", config)
	assert.Nil(t, err)

	assert.Nil(t, parser.ParseArgs([]string{"app", "--server.host", "0.0.0.0", "--server.port=9090"}))
	assert.Equal(t, "0.0.0.0", config.server.Host)
	assert.Equal(t, 9090, config.server.Port)
}

func TestParseOSArgs(t *testing.T) {
	args := os.Args
	defer func() { os.Args = args }()
	os.Args = []string{"app", "--server.port", "1234"}

	config := newAppConfig()
	parser, err := Generate("app", config)
	assert.Nil(t, err)

	assert.Nil(t, parser.Parse())
	assert.Equal(t, 1234, config.server.Port)
}

func TestParseEnv(t *testing.T) {
	t.Setenv("APP_SERVER_HOST", "example.com")
	t.Setenv("APP_SERVER_PORT", "7070")

	config := newAppConfig()
	parser, err := Generate("app", config)
	assert.Nil(t, err)

	assert.Nil(t, parser.ParseArgs([]string{"app"}))
	assert.Equal(t, "example.com", config.server.Host)
	assert.Equal(t, 7070, config.server.Port)
}

func TestParseFlagOverridesEnv(t *testing.T) {
	t.Setenv("APP_SERVER_PORT", "7070")

	config := newAppConfig()
	parser, err := Generate("app", config)
	assert.Nil(t, err)

	assert.Nil(t, parser.ParseArgs([]string{"app", "--server.port", "6060"}))
	assert.Equal(t, 6060, config.server.Port)
}

func TestParseYamlFile(t *testing.T) {
	p := writeFile(t, "config.yaml", "server:\n  host: yaml.example\n  port: 5050\n")

	config := newAppConfig()
	parser, err := Generate("app", config)
	assert.Nil(t, err)

	assert.Nil(t, parser.ParseArgs([]string{"app", "--config.path", p}))
	assert.Equal(t, p, parser.file.Path)
	assert.Equal(t, "yaml.example", config.server.Host)
	assert.Equal(t, 5050, config.server.Port)
}

func TestParseTomlFileWithFlagOverride(t *testing.T) {
	p := writeFile(t, "config.toml", "[server]\nhost = \"toml.example\"\nport = 4040\n")

	config := newAppConfig()
	parser, err := Generate("app", config)
	assert.Nil(t, err)

	assert.Nil(t, parser.ParseArgs([]string{"app", "--config.path", p, "--server.port", "4141"}))
	assert.Equal(t, "toml.example", config.server.Host)
	assert.Equal(t, 4141, config.server.Port)
}

func TestParseConfigPathFromEnv(t *testing.T) {
	p := writeFile(t, "config.yaml", "server:\n  port: 3030\n")
	t.Setenv("APP_CONFIG_PATH", p)

	config := newAppConfig()
	parser, err := Generate("app", config)
	assert.Nil(t, err)

	assert.Nil(t, parser.ParseArgs([]string{"app"}))
	assert.Equal(t, 3030, config.server.Port)
}

func TestParseFileErrors(t *testing.T) {
	cases := map[string]string{
		"unsupported extension": writeFile(t, "config.json", "{}"),
		"missing file":          filepath.Join(t.TempDir(), "missing.yaml"),
		"invalid content":       writeFile(t, "invalid.yaml", "server: [unclosed\n"),
	}

	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			parser, err := Generate("app", newAppConfig())
			assert.Nil(t, err)
			assert.Error(t, parser.ParseArgs([]string{"app", "--config.path", p}))
		})
	}
}

func TestParseTwice(t *testing.T) {
	parser, err := Generate("app", newAppConfig())
	assert.Nil(t, err)

	assert.Nil(t, parser.ParseArgs([]string{"app"}))
	assert.Equal(t, ErrAlreadyParsed, parser.ParseArgs([]string{"app"}))
}

func TestParseUnknownFlag(t *testing.T) {
	parser, err := Generate("app", newAppConfig())
	assert.Nil(t, err)

	err = parser.ParseArgs([]string{"app", "--unknown"})
	assert.IsType(t, ErrParseFlags{}, err)
	assert.Contains(t, err.Error(), "failed to parse flags")
}

func TestParseConfigureError(t *testing.T) {
	configureErr := errors.New("configure failed")
	config := newAppConfig()
	config.server.configureErr = configureErr
	parser, err := Generate("app", config)
	assert.Nil(t, err)

	assert.Equal(t, configureErr, parser.ParseArgs([]string{"app"}))
}

func TestGenerateBindError(t *testing.T) {
	config := newAppConfig()
	config.server.bindErr = errors.New("bind failed")

	parser, err := Generate("app", config)
	assert.Nil(t, parser)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bind failed")
}

func TestUsage(t *testing.T) {
	parser, err := Generate("app", newAppConfig())
	assert.Nil(t, err)

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	assert.Nil(t, err)
	defer func() { _ = devNull.Close() }()
	parser.cmd.SetOut(devNull)
	parser.cmd.SetErr(devNull)

	assert.Nil(t, parser.Usage())
}
