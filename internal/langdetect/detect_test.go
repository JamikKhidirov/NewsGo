package langdetect

import "testing"

func TestDetect(t *testing.T) {
	cases := map[string]string{
		"Путин встретился с президентом": "ru",
		"Breaking news from Washington":  "en",
		"12345 !!!":                      "",
		"AI стартап привлёк инвестиции":  "ru",
	}
	for text, want := range cases {
		if got := Detect(text); got != want {
			t.Errorf("Detect(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	if Normalize("ru-RU") != "ru" {
		t.Fatal("ru-RU must normalize to ru")
	}
	if Normalize("EN_us") != "en" {
		t.Fatal("EN_us must normalize to en")
	}
}
