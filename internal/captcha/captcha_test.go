package captcha

import (
	"bytes"
	"image/png"
	"testing"
	"time"
)

func TestGenerateAndValidate(t *testing.T) {
	s := New(5 * time.Minute)

	id, err := s.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	a, b, ok := s.GetChallenge(id)
	if !ok {
		t.Fatal("expected challenge to exist")
	}

	// Correct answer succeeds.
	if !s.Validate(id, a+b) {
		t.Error("expected correct answer to validate")
	}
}

func TestValidateWrongAnswer(t *testing.T) {
	s := New(5 * time.Minute)

	id, err := s.Generate()
	if err != nil {
		t.Fatal(err)
	}

	a, b, _ := s.GetChallenge(id)
	wrong := a + b + 1

	if s.Validate(id, wrong) {
		t.Error("expected wrong answer to fail")
	}
}

func TestValidateOneTimeUse(t *testing.T) {
	s := New(5 * time.Minute)

	id, err := s.Generate()
	if err != nil {
		t.Fatal(err)
	}

	a, b, _ := s.GetChallenge(id)
	answer := a + b

	if !s.Validate(id, answer) {
		t.Fatal("first validate should succeed")
	}

	// Second use should fail — challenge is consumed.
	if s.Validate(id, answer) {
		t.Error("expected second validate to fail (one-time use)")
	}
}

func TestValidateExpired(t *testing.T) {
	s := New(1 * time.Millisecond)

	id, err := s.Generate()
	if err != nil {
		t.Fatal(err)
	}

	a, b, _ := s.GetChallenge(id)
	time.Sleep(5 * time.Millisecond)

	if s.Validate(id, a+b) {
		t.Error("expected expired challenge to fail")
	}
}

func TestGetChallengeExpired(t *testing.T) {
	s := New(1 * time.Millisecond)

	id, err := s.Generate()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Millisecond)

	_, _, ok := s.GetChallenge(id)
	if ok {
		t.Error("expected expired challenge to return ok=false")
	}
}

func TestGetChallengeNotFound(t *testing.T) {
	s := New(5 * time.Minute)

	_, _, ok := s.GetChallenge("nonexistent")
	if ok {
		t.Error("expected missing challenge to return ok=false")
	}
}

func TestCleanup(t *testing.T) {
	s := New(1 * time.Millisecond)

	_, err := s.Generate()
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Generate()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Millisecond)
	s.Cleanup()

	s.mu.Lock()
	count := len(s.entries)
	s.mu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 entries after cleanup, got %d", count)
	}
}

func TestRenderPNG(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderPNG(&buf, 3, 7); err != nil {
		t.Fatal(err)
	}

	if buf.Len() == 0 {
		t.Fatal("expected non-empty PNG output")
	}

	img, err := png.Decode(&buf)
	if err != nil {
		t.Fatalf("output is not a valid PNG: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() == 0 || bounds.Dy() == 0 {
		t.Error("expected non-zero image dimensions")
	}
}

func TestDigitRange(t *testing.T) {
	s := New(5 * time.Minute)

	for i := 0; i < 50; i++ {
		id, err := s.Generate()
		if err != nil {
			t.Fatal(err)
		}
		a, b, ok := s.GetChallenge(id)
		if !ok {
			t.Fatal("expected challenge to exist")
		}
		if a < 1 || a > 9 || b < 1 || b > 9 {
			t.Errorf("digits out of range: a=%d b=%d", a, b)
		}
	}
}
