package companyname

import (
	"context"
	"testing"
)

type fakeText map[string]string

func (f fakeText) GetText(_ context.Context, url string) (string, error) { return f[url], nil }

func TestTitleResolver(t *testing.T) {
	getter := fakeText{
		"https://lbresearch.pinpointhq.com": `<html><head><title>Jobs at Centellic | Centellic Careers</title></head></html>`,
		"https://empty.pinpointhq.com":      `<html><head><title>Just a moment...</title></head></html>`,
	}
	r := newTitleResolver(getter, "https://%s.pinpointhq.com", ExtractTitleName)

	if got, _ := r.Name(context.Background(), "lbresearch"); got != "Centellic" {
		t.Errorf("Name(lbresearch) = %q, want Centellic", got)
	}
	if got, _ := r.Name(context.Background(), "empty"); got != "" {
		t.Errorf("Name(empty) = %q, want empty", got)
	}
}

// Lever's storefront titles the page with the bare company name — no lead-in, no suffix.
// Live-verified against jobs.lever.co/binance, whose <title> is literally "Binance".
func TestLeverResolverUsesTheBareTitle(t *testing.T) {
	getter := fakeText{
		"https://jobs.lever.co/binance": `<html><head><title>Binance</title></head></html>`,
	}
	reg := NewRegistry(getter)
	if got, _ := reg["lever"].Name(context.Background(), "binance"); got != "Binance" {
		t.Errorf("Name(binance) = %q, want Binance", got)
	}
}

// Ashby's storefront titles itself "{Name} Jobs" — not the Pinpoint "Careers" suffix.
// Live-verified against jobs.ashbyhq.com/airgarage, whose <title> is "AirGarage Jobs".
func TestAshbyResolverUsesTheJobsSuffix(t *testing.T) {
	getter := fakeText{
		"https://jobs.ashbyhq.com/airgarage": `<html><head><title>AirGarage Jobs</title></head></html>`,
	}
	reg := NewRegistry(getter)
	if got, _ := reg["ashby"].Name(context.Background(), "airgarage"); got != "AirGarage" {
		t.Errorf("Name(airgarage) = %q, want AirGarage", got)
	}
}

func TestRegistryLookup(t *testing.T) {
	reg := NewRegistry(fakeText{})
	for _, src := range []string{"pinpoint", "lever", "ashby"} {
		if _, ok := reg[src]; !ok {
			t.Errorf("registry missing %s resolver", src)
		}
	}
	// Greenhouse job URLs are vanity domains, so it has no URL-derivable board.
	if _, ok := reg["greenhouse"]; ok {
		t.Error("registry should not have a greenhouse resolver")
	}
	// BambooHR's careers page is a client-rendered SPA: the static <title> is always the
	// platform's own boilerplate, never the tenant's name, so no resolver can work here.
	if _, ok := reg["bamboohr"]; ok {
		t.Error("registry should not have a bamboohr resolver — its static title never carries the name")
	}
	if _, ok := reg["nonexistent-ats"]; ok {
		t.Error("registry should not have a resolver for an unknown source")
	}
}
