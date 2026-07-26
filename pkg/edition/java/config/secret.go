package config

import "gopkg.in/yaml.v3"

// Secret is a forwarding credential that redacts itself in formatted output.
type Secret string

func (Secret) String() string   { return "[REDACTED]" }
func (Secret) GoString() string { return "[REDACTED]" }

func (f *Forwarding) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		Mode              ForwardingMode `yaml:"mode"`
		VelocitySecret    Secret         `yaml:"velocitySecret"`
		BungeeGuardSecret Secret         `yaml:"bungeeGuardSecret"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	f.Mode = raw.Mode
	f.VelocitySecret = raw.VelocitySecret
	f.BungeeGuardSecret = raw.BungeeGuardSecret
	return nil
}

func (f Forwarding) MarshalYAML() (any, error) {
	return struct {
		Mode ForwardingMode `yaml:"mode,omitempty"`
	}{Mode: f.Mode}, nil
}
