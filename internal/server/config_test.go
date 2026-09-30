package server

import "testing"

func TestConfigValidate(t *testing.T) {
	ok := DefaultConfig()
	if err := ok.Validate(); err != nil {
		t.Fatalf("mặc định phải hợp lệ: %v", err)
	}
	cases := map[string]func(*Config){
		"http trên 0.0.0.0":     func(c *Config) { c.Listen = "0.0.0.0:9443" },
		"http trên mọi địa chỉ": func(c *Config) { c.Listen = ":9443" },
		"chỉ có cert":           func(c *Config) { c.TLSCert = "/x.pem" },
		"idle = 0":              func(c *Config) { c.SessionIdle = 0 },
		"max < idle":            func(c *Config) { c.SessionMax = c.SessionIdle / 2 },
		"listen hỏng":           func(c *Config) { c.Listen = "khong-co-cong" },
	}
	for name, mut := range cases {
		c := DefaultConfig()
		mut(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: phải lỗi", name)
		}
	}
	tls := DefaultConfig()
	tls.Listen, tls.TLSCert, tls.TLSKey = "0.0.0.0:9443", "/c.pem", "/k.pem"
	if err := tls.Validate(); err != nil {
		t.Errorf("TLS trên 0.0.0.0 phải hợp lệ: %v", err)
	}
	for _, l := range []string{"127.0.0.1:9090", "[::1]:9090", "localhost:9090"} {
		c := DefaultConfig()
		c.Listen = l
		if err := c.Validate(); err != nil {
			t.Errorf("%s không TLS phải hợp lệ: %v", l, err)
		}
	}
}
