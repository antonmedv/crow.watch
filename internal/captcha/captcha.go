package captcha

import (
	"crypto/rand"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math/big"
	"sync"
	"time"
)

type challenge struct {
	a, b      int
	answer    int
	expiresAt time.Time
}

// Store holds pending CAPTCHA challenges in memory.
type Store struct {
	mu      sync.Mutex
	entries map[string]*challenge
	ttl     time.Duration
}

// New creates a Store with the given challenge TTL.
func New(ttl time.Duration) *Store {
	return &Store{
		entries: make(map[string]*challenge),
		ttl:     ttl,
	}
}

// Generate creates a new CAPTCHA challenge and returns its ID.
func (s *Store) Generate() (string, error) {
	a, err := cryptoRandInt(1, 9)
	if err != nil {
		return "", err
	}
	b, err := cryptoRandInt(1, 9)
	if err != nil {
		return "", err
	}

	idBytes := make([]byte, 18)
	if _, err := rand.Read(idBytes); err != nil {
		return "", err
	}
	id := base64.URLEncoding.EncodeToString(idBytes)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[id] = &challenge{
		a:         a,
		b:         b,
		answer:    a + b,
		expiresAt: time.Now().Add(s.ttl),
	}
	return id, nil
}

// GetChallenge returns the two operands for the given ID without consuming it.
func (s *Store) GetChallenge(id string) (a, b int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, exists := s.entries[id]
	if !exists || time.Now().After(c.expiresAt) {
		return 0, 0, false
	}
	return c.a, c.b, true
}

// Validate checks the answer and deletes the challenge (one-time use).
func (s *Store) Validate(id string, answer int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, exists := s.entries[id]
	if !exists || time.Now().After(c.expiresAt) {
		delete(s.entries, id)
		return false
	}
	delete(s.entries, id)
	return c.answer == answer
}

// Cleanup removes expired entries.
func (s *Store) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, c := range s.entries {
		if now.After(c.expiresAt) {
			delete(s.entries, id)
		}
	}
}

// StartCleanup runs Cleanup periodically until done is closed.
func (s *Store) StartCleanup(interval time.Duration, done <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.Cleanup()
			case <-done:
				return
			}
		}
	}()
}

// cryptoRandInt returns a random int in [min, max] using crypto/rand.
func cryptoRandInt(min, max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()) + min, nil
}

// 5x7 bitmap font for digits 0-9 and '+', '=', '?'.
var glyphs = map[rune][7]uint8{
	'0': {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E},
	'1': {0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'2': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1F},
	'3': {0x0E, 0x11, 0x01, 0x06, 0x01, 0x11, 0x0E},
	'4': {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02},
	'5': {0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E},
	'6': {0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E},
	'7': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
	'8': {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E},
	'9': {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C},
	'+': {0x00, 0x04, 0x04, 0x1F, 0x04, 0x04, 0x00},
	'=': {0x00, 0x00, 0x1F, 0x00, 0x1F, 0x00, 0x00},
	'?': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x00, 0x04},
	' ': {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
}

const (
	glyphW  = 5
	glyphH  = 7
	scale   = 4
	gap     = 1 // gap between characters in glyph units
	padding = 8 // pixels of padding around the text
)

// RenderPNG draws "a + b = ?" as a PNG image.
func RenderPNG(w io.Writer, a, b int) error {
	text := digitStr(a) + " + " + digitStr(b) + " = ?"

	charW := (glyphW + gap) * scale
	textW := len(text)*charW - gap*scale
	textH := glyphH * scale
	imgW := textW + 2*padding
	imgH := textH + 2*padding

	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))
	bg := color.RGBA{245, 245, 245, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	fg := color.RGBA{50, 50, 50, 255}

	for i, ch := range text {
		glyph, ok := glyphs[ch]
		if !ok {
			continue
		}
		ox := padding + i*charW
		for row := 0; row < glyphH; row++ {
			bits := glyph[row]
			for col := 0; col < glyphW; col++ {
				if bits&(1<<(glyphW-1-col)) != 0 {
					for dy := 0; dy < scale; dy++ {
						for dx := 0; dx < scale; dx++ {
							img.Set(ox+col*scale+dx, padding+row*scale+dy, fg)
						}
					}
				}
			}
		}
	}

	return png.Encode(w, img)
}

func digitStr(n int) string {
	if n >= 10 {
		return string(rune('0'+n/10)) + string(rune('0'+n%10))
	}
	return string(rune('0' + n))
}
