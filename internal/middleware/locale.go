package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

// LocaleMiddleware picks a locale from (in order): ?lang=, Accept-Language header,
// or the configured default. It stores the code in ctx.Locals("locale").
// Supported codes: en, hi, hinglish, ta, te, kn, mr, gu, bn, or, ml, pa.
var supportedLanguages = map[string]bool{
	"en": true, "hi": true, "hinglish": true,
	"ta": true, "te": true, "kn": true,
	"mr": true, "gu": true, "bn": true,
	"or": true, "ml": true, "pa": true,
}

func LocaleMiddleware(defaultLocale string) fiber.Handler {
	if defaultLocale == "" {
		defaultLocale = "en"
	}
	return func(c fiber.Ctx) error {
		lang := strings.ToLower(strings.TrimSpace(c.Query("lang")))
		if lang == "" {
			ah := c.Get("Accept-Language")
			if ah != "" {
				// Pick first tag before comma/semicolon.
				parts := strings.FieldsFunc(ah, func(r rune) bool { return r == ',' || r == ';' })
				if len(parts) > 0 {
					primary := strings.TrimSpace(parts[0])
					if idx := strings.Index(primary, "-"); idx != -1 {
						primary = primary[:idx]
					}
					lang = strings.ToLower(primary)
				}
			}
		}
		if !supportedLanguages[lang] {
			lang = defaultLocale
		}
		c.Locals("locale", lang)
		return c.Next()
	}
}
