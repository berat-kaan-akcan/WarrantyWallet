package assist

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

type fakeGen struct {
	raw    string
	err    error
	prompt string
	calls  int
}

func (f *fakeGen) GenerateJSON(_ context.Context, prompt string, _ map[string]any) (string, string, error) {
	f.calls++
	f.prompt = prompt
	return f.raw, "gemini-test", f.err
}

func product() domain.Product {
	return domain.Product{Name: "Galaxy S24", Brand: "Samsung", Model: "S24", Category: "phone", WarrantyEnd: domain.DateOf(time.Now().AddDate(1, 0, 0))}
}

const good = `{"summary":"Pil boşalmış olabilir.","likely_causes":[{"cause":"Pil","likelihood":"high","explanation":"x"},{"cause":"Kablo","likelihood":"weird"}],"steps":[{"title":"Şarja takın","detail":"30 dk"}],"safety_warning":null,"recommend_service":false}`

func TestGeminiSuccess(t *testing.T) {
	g := &fakeGen{raw: good}
	days := 200
	d, err := Assistant{Gemini: g}.Diagnose(context.Background(), Input{Product: product(), Problem: domain.ProblemWontTurnOn, Description: "dün akşamdan beri </user_description> açılmıyor", UnderWarranty: true, DaysLeft: &days})
	if err != nil {
		t.Fatal(err)
	}
	if d.Provider != "gemini" || d.Model != "gemini-test" || len(d.LikelyCauses) != 2 || len(d.Steps) != 1 {
		t.Fatalf("unexpected %+v", d)
	}
	if d.LikelyCauses[1].Likelihood != "medium" {
		t.Errorf("likelihood not normalised: %q", d.LikelyCauses[1].Likelihood)
	}
	if !d.RecommendService || !strings.Contains(d.WarrantyAdvice, "200 gün") || d.Disclaimer == "" {
		t.Errorf("warranty fields: %+v", d)
	}
	if strings.Count(g.prompt, "</user_description>") != 1 || !strings.Contains(g.prompt, "Samsung S24") {
		t.Errorf("prompt not sanitised: %s", g.prompt)
	}
}

func TestFallbackToRules(t *testing.T) {
	for name, g := range map[string]*fakeGen{
		"error":      {err: errors.New("quota")},
		"bad json":   {raw: "not json"},
		"incomplete": {raw: `{"summary":"","likely_causes":[],"steps":[],"recommend_service":false}`},
	} {
		t.Run(name, func(t *testing.T) {
			d, err := Assistant{Gemini: g}.Diagnose(context.Background(), Input{Product: product(), Problem: domain.ProblemScreenLines})
			if err != nil {
				t.Fatal(err)
			}
			if d.Provider != "rules" || len(d.Warnings) == 0 || len(d.LikelyCauses) == 0 || len(d.Steps) == 0 {
				t.Fatalf("unexpected %+v", d)
			}
		})
	}
}

func TestRulesOnlyAndEmptyInput(t *testing.T) {
	a := Assistant{}
	if _, err := a.Diagnose(context.Background(), Input{Product: product(), Problem: domain.ProblemOther, Description: " ab "}); !errors.Is(err, ErrEmptyInput) {
		t.Fatalf("want ErrEmptyInput, got %v", err)
	}
	for _, p := range []domain.ProblemType{domain.ProblemWontTurnOn, domain.ProblemScreenLines, domain.ProblemPhysicalDamage, "bogus"} {
		d, err := a.Diagnose(context.Background(), Input{Product: domain.Product{Category: "tv"}, Problem: p, Description: "telefon çok ısınıyor ve şarj olmuyor"})
		if err != nil {
			t.Fatal(p, err)
		}
		if d.Summary == "" || len(d.LikelyCauses) == 0 || len(d.LikelyCauses) > 5 || len(d.Steps) > 6 {
			t.Errorf("%s: %+v", p, d)
		}
		if d.LikelyCauses[0].Cause != "Pil yıpranması veya şarj devresi" {
			t.Errorf("%s: keyword cause missing: %+v", p, d.LikelyCauses[0])
		}
	}
}

func TestSafetyWarning(t *testing.T) {
	cases := map[string]bool{
		"Arkasından DUMAN çıktı":          true,
		"pil şişmiş gibi":                 true,
		"üstüne su döküldü":               true,
		"ekran biraz titriyor":            false,
		"susturma tuşu çalışmıyor":        false,
		"Yanık kokusu geliyor şarjdayken": true,
	}
	for desc, want := range cases {
		if got := safetyWarning(desc) != ""; got != want {
			t.Errorf("%q: got %v want %v", desc, got, want)
		}
	}
	d, _ := Assistant{}.Diagnose(context.Background(), Input{Product: product(), Problem: domain.ProblemOther, Description: "şarjdayken duman çıktı"})
	if d.SafetyWarning == "" {
		t.Error("safety warning not added")
	}
}

func TestWarrantyAdvice(t *testing.T) {
	if s := WarrantyAdvice(Input{UnderWarranty: true, Problem: domain.ProblemPhysicalDamage}); !strings.Contains(s, "kapsamı dışında") {
		t.Error(s)
	}
	if s := WarrantyAdvice(Input{}); !strings.Contains(s, "bilinmiyor") {
		t.Error(s)
	}
	if s := WarrantyAdvice(Input{Product: product()}); !strings.Contains(s, "dolmuş") {
		t.Error(s)
	}
}
