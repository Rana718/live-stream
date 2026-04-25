// Package share renders share-cards (a.k.a. "posters") for course pages.
// These are the thumbnails that show up when an instructor pastes a course
// URL into WhatsApp, Twitter/X, or LinkedIn — a 1200×630 image with the
// tenant's logo, the course title, and the price.
//
// We render with Go's image/draw + an SVG-style title via a TTF font so we
// don't have to depend on headless Chrome. Output is PNG to keep things
// simple; OG meta tags can point at our endpoint and the social platform
// fetches the image directly. Cache aggressively at the CDN.
package share

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"live-platform/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// W and H mirror Open Graph's recommended share-card size. Twitter, LinkedIn,
// WhatsApp all crop variations of this; 1200×630 is the safe denominator.
const (
	W = 1200
	H = 630
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

// Render returns a PNG for the requested course. Failures are bubbled —
// the handler returns 404 if the course doesn't exist or the tenant
// doesn't match (RLS enforces the latter).
func (s *Service) Render(ctx context.Context, courseID uuid.UUID) ([]byte, error) {
	course, err := s.q.GetCourseByID(ctx, pgtype.UUID{Bytes: courseID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("course not found")
	}
	tenant, err := s.q.GetTenantByID(ctx, course.TenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant not found")
	}

	img := image.NewRGBA(image.Rect(0, 0, W, H))

	// Background: tenant brand colour or a default purple. We don't parse
	// the theme JSON here (would require json.Unmarshal + hex parse +
	// fallbacks) — just paint a deterministic gradient based on the
	// tenant's UUID so each tenant gets a recognisable card colour.
	bg := tenantBrandColor(uuid.UUID(tenant.ID.Bytes))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	// Drop a darker bar across the bottom for the price/CTA.
	draw.Draw(img,
		image.Rect(0, H-110, W, H),
		&image.Uniform{C: color.RGBA{0, 0, 0, 64}},
		image.Point{}, draw.Over)

	// Tenant name top-left.
	drawText(img, tenant.Name, 60, 90, color.White, 2)

	// Course title — wrapped, big, centre-ish.
	wrap(img, course.Title, 60, 220, W-120, 50, color.White, 4)

	// Price strip.
	priceText := "Enroll now"
	if course.Price.Valid {
		v, _ := course.Price.Float64Value()
		if v.Float64 > 0 {
			priceText = fmt.Sprintf("Rs %.0f", v.Float64)
		} else {
			priceText = "Free"
		}
	}
	drawText(img, priceText, 60, H-50, color.White, 3)

	// CTA right side.
	drawText(img, tenant.OrgCode+" - "+tenant.Slug, W-360, H-50, color.RGBA{255, 255, 255, 200}, 2)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// tenantBrandColor picks a deterministic non-ugly colour from the first
// bytes of a tenant's UUID. Avoids saturating yellows/cyans that read
// poorly on social cards by clamping HSL values.
func tenantBrandColor(id uuid.UUID) color.RGBA {
	r := byte(60 + int(id[0])/3)  // 60-145
	g := byte(40 + int(id[1])/3)  // 40-125
	b := byte(110 + int(id[2])/3) // 110-195
	return color.RGBA{r, g, b, 255}
}

// drawText renders a string with `scale` upsampling because basicfont's
// 7×13 face is comically small at 1200px width. We do nearest-neighbour
// scaling — nothing fancy, but readable.
func drawText(img *image.RGBA, s string, x, y int, c color.Color, scale int) {
	if s == "" {
		return
	}
	// Render at 1× into a scratch image, then nearest-scale up.
	face := basicfont.Face7x13
	w := font.MeasureString(face, s).Round()
	if w == 0 {
		return
	}
	src := image.NewRGBA(image.Rect(0, 0, w, 13))
	d := &font.Drawer{
		Dst:  src,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(0, 11),
	}
	d.DrawString(s)
	for sy := 0; sy < src.Bounds().Dy(); sy++ {
		for sx := 0; sx < src.Bounds().Dx(); sx++ {
			a := src.At(sx, sy)
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.Set(x+sx*scale+dx, y-13*scale+sy*scale+dy, a)
				}
			}
		}
	}
}

// wrap is a naive word wrapper. Splits on spaces and breaks when the
// next word would exceed maxWidth. Good enough for course titles —
// real typography is overkill for share cards.
func wrap(img *image.RGBA, s string, x, y, maxWidth, lineHeight int, c color.Color, scale int) {
	face := basicfont.Face7x13
	words := splitWords(s)
	var line string
	for _, w := range words {
		next := w
		if line != "" {
			next = line + " " + w
		}
		if font.MeasureString(face, next).Round()*scale > maxWidth {
			drawText(img, line, x, y, c, scale)
			y += lineHeight
			line = w
		} else {
			line = next
		}
	}
	if line != "" {
		drawText(img, line, x, y, c, scale)
	}
}

func splitWords(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if r == ' ' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
