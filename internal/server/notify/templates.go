package notify

import (
	"bytes"
	"embed"
	"html/template"
)

//go:embed templates/email.html
var emailTemplateFS embed.FS

var emailTmpl = template.Must(template.ParseFS(emailTemplateFS, "templates/email.html"))

// EmailData holds the data for rendering the HTML email template.
type EmailData struct {
	Subject   string
	Fields    []KeyValue
	Timestamp string
	AppURL    string
}

// KeyValue represents a single key-value pair for the email template table.
type KeyValue struct {
	Key   string
	Value string
}

// RenderEmail renders the HTML email template with the given data.
func RenderEmail(data EmailData) (string, error) {
	var buf bytes.Buffer
	if err := emailTmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
