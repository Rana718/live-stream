package config

import "testing"

func baseProd() *Config {
	c := &Config{}
	c.Server.Env = "production"
	c.JWT.AccessSecret = "a-very-long-access-secret-value-32ch!!"
	c.JWT.RefreshSecret = "a-very-long-refresh-secret-value-32c!!"
	c.Database.Password = "s3cret-db-pass"
	c.Database.SSLMode = "require"
	c.OTP.DevMode = false
	c.SMS.Provider = "msg91"
	c.SMS.AuthKey = "k"
	c.Razorpay.KeyID = "rzp_live_x"
	c.Razorpay.KeySecret = "secret"
	c.Razorpay.WebhookSecret = "whsec"
	c.MinIO.AccessKey = "prodkey"
	c.MinIO.SecretKey = "prodsecret"
	c.CORS.AllowedOrigins = []string{"https://app.example.com"}
	c.Google.ClientIDs = []string{"123.apps.googleusercontent.com"}
	return c
}

func TestValidate_ProductionHappyPath(t *testing.T) {
	if err := baseProd().validate(); err != nil {
		t.Fatalf("expected valid prod config, got: %v", err)
	}
}

func TestValidate_RejectsDevOTPInProduction(t *testing.T) {
	c := baseProd()
	c.OTP.DevMode = true
	if err := c.validate(); err == nil {
		t.Fatal("expected OTP_DEV_MODE=true to be rejected in production")
	}
}

func TestValidate_RejectsWeakJWT(t *testing.T) {
	c := baseProd()
	c.JWT.AccessSecret = "access-secret-key"
	if err := c.validate(); err == nil {
		t.Fatal("expected weak JWT secret to be rejected")
	}
}

func TestValidate_RejectsWildcardCORS(t *testing.T) {
	c := baseProd()
	c.CORS.AllowedOrigins = []string{"*"}
	if err := c.validate(); err == nil {
		t.Fatal("expected wildcard CORS to be rejected in production")
	}
}

func TestValidate_RejectsPlaintextDB(t *testing.T) {
	c := baseProd()
	c.Database.SSLMode = "disable"
	if err := c.validate(); err == nil {
		t.Fatal("expected sslmode=disable to be rejected in production")
	}
}

func TestValidate_RejectsMissingRazorpay(t *testing.T) {
	c := baseProd()
	c.Razorpay.KeySecret = ""
	if err := c.validate(); err == nil {
		t.Fatal("expected missing Razorpay secret to be rejected")
	}
}

func TestValidate_DevModeSkipsHardChecks(t *testing.T) {
	c := &Config{}
	c.Server.Env = "development"
	c.CORS.AllowedOrigins = []string{"*"}
	c.OTP.DevMode = true
	if err := c.validate(); err != nil {
		t.Fatalf("dev config should be permissive, got: %v", err)
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(" a , b ,,c ")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if splitList("   ") != nil {
		t.Fatal("blank should yield nil")
	}
}
