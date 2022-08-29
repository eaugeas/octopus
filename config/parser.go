package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Config interface {
	Use() string
	EnvPrefix() string
	Binders() []Binder
}

type Parser struct {
	Config Config

	file *ConfigFile

	cmd *cobra.Command
	v   *viper.Viper
}

func (p *Parser) Parse() error {
	if p.cmd.PersistentFlags().Parsed() {
		return ErrAlreadyParsed
	}

	if err := p.cmd.PersistentFlags().Parse(os.Args); err != nil {
		return ErrParseFlags{err}
	}

	// keep file first so that any parameters read from the file are used
	// as defaults for the other flags
	var binders []Binder
	binders = append(binders, p.file)
	binders = append(binders, p.Config.Binders()...)

	for _, c := range binders {
		if err := c.Configure(p.v); err != nil {
			return err
		}
	}

	return nil
}

func (p *Parser) Usage() error {
	return p.cmd.Usage()
}

func Generate(app string, config Config) (*Parser, error) {
	v := viper.New()
	// all environment variables start with prefix `prefix` and are set
	// by replacing `.` and `_` to _.
	v.SetEnvPrefix(app)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	cmd := &cobra.Command{Use: app}
	file := ConfigFile{}
	var binders []Binder
	binders = append(binders, &file)
	binders = append(binders, config.Binders()...)

	for _, c := range binders {
		if err := c.Bind(v, cmd); err != nil {
			return nil, fmt.Errorf("failed to bind flags %s", err.Error())
		}
	}

	if err := v.BindPFlags(cmd.PersistentFlags()); err != nil {
		return nil, fmt.Errorf("failed to bind flags %s", err.Error())
	}

	return &Parser{file: &file, Config: config, cmd: cmd, v: v}, nil
}
