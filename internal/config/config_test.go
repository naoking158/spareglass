package config

import "testing"

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Config){
		"fps":   func(c *Config) { c.FPS = 0 },
		"scale": func(c *Config) { c.RenderScale = 10 },
		"token": func(c *Config) { c.Token = "short" },
	}
	for name, mutate := range cases {
		c := Default()
		mutate(&c)
		if c.Validate() == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
