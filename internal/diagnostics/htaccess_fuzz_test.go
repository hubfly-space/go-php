package diagnostics

import "testing"

func FuzzHtaccessTranslator(f *testing.F) {
	f.Add("RewriteEngine On\nRewriteRule ^(.*)$ index.php/$1 [L]")
	f.Add("Redirect 301 /old /new")
	f.Add("RewriteCond %{REQUEST_FILENAME} !-f\nRewriteRule ^ index.php [L]")

	f.Fuzz(func(t *testing.T, input string) {
		translator := NewHtaccessTranslator()
		_, _ = translator.Translate(input)
	})
}
