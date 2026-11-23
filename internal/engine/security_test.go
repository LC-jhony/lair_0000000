package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/lair-sh/lair/internal/config"
)

func TestEngineSharesConfigStoreWithInstaller(t *testing.T) {
	e, _ := testEnv(t)
	if e.Installer.ConfigStore() != e.Config {
		t.Fatal("installer and engine use different config stores")
	}
	// Both writers must be serialised by one mutex: with two stores a load
	// from disk can be overwritten by a stale write from the other one.
	var wg sync.WaitGroup
	stores := []*config.Store{e.Config, e.Installer.ConfigStore()}
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store := stores[i%len(stores)]
			err := store.Update(func(c *config.Config) error {
				c.Defaults[fmt.Sprintf("key-%02d", i)] = "value"
				return nil
			})
			if err != nil {
				t.Errorf("update %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	c, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		key := fmt.Sprintf("key-%02d", i)
		if c.Defaults[key] != "value" {
			t.Fatalf("lost concurrent update for %s", key)
		}
	}
}

func TestWebControlInvalidConfigLeavesServerUnchanged(t *testing.T) {
	e, _ := testEnv(t)
	before := e.WebSettings()
	// Equal ports are rejected by Normalize before anything is persisted or
	// applied to the in-memory listener.
	_, err := e.WebControl(context.Background(), WebRequest{
		Op: "configure", HTTPPort: 19080, HTTPSPort: 19080,
		Bind: "0.0.0.0", Domain: "other.test",
	})
	if err == nil {
		t.Fatal("invalid web settings were accepted")
	}
	after, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	if after.Web.HTTPPort != before.HTTPPort || after.Web.HTTPSPort != before.HTTPSPort ||
		after.Web.Bind != before.Bind || after.Web.Domain != before.Domain {
		t.Fatalf("persisted settings changed: %+v (before %+v)", after.Web, before)
	}
	if e.Web.HTTPPort != before.HTTPPort || e.Web.HTTPSPort != before.HTTPSPort ||
		e.Web.Domain != before.Domain || e.Web.Bind != before.Bind {
		t.Fatalf("in-memory server changed: port=%d/%d domain=%q bind=%q",
			e.Web.HTTPPort, e.Web.HTTPSPort, e.Web.Domain, e.Web.Bind)
	}
}

func TestWebStatusWarnsOnLanBind(t *testing.T) {
	e, _ := testEnv(t)
	res, err := e.WebControl(context.Background(), WebRequest{Op: "configure", Bind: "0.0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if e.Web.Bind != "0.0.0.0" {
		t.Fatalf("bind not applied to Go server: %q", e.Web.Bind)
	}
	if !strings.Contains(res.Web.Warning, "LAN") {
		t.Fatalf("no LAN exposure warning: %+v", res.Web)
	}
}

func TestWebControlConcurrentConfigure(t *testing.T) {
	e, _ := testEnv(t)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			port := 18080 + i
			if _, err := e.WebControl(context.Background(), WebRequest{
				Op: "configure", HTTPPort: port, HTTPSPort: port + 1000,
			}); err != nil {
				t.Errorf("configure %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	c, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	if e.Web.HTTPPort != c.Web.HTTPPort || e.Web.HTTPSPort != c.Web.HTTPSPort {
		t.Fatalf("server and config out of sync: %d/%d vs %+v",
			e.Web.HTTPPort, e.Web.HTTPSPort, c.Web)
	}
}
