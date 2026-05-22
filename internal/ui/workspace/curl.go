package workspace

import (
	"strings"
	"tracto/internal/model"
	"tracto/internal/ui/settings"
	"tracto/internal/utils"
)

func BuildCurlCommand(t *RequestTab, env map[string]string) string {
	urlRaw := strings.ReplaceAll(t.URLInput.Text(), "\n", "")
	urlRaw = strings.ReplaceAll(urlRaw, "\t", "")
	urlRaw = strings.TrimSpace(utils.SanitizeText(urlRaw))
	rawURL := processTemplate(urlRaw, env)
	if rawURL == "" {
		return ""
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "http://" + rawURL
	}

	method := strings.ToUpper(strings.TrimSpace(t.Method))
	if method == "" {
		method = "GET"
	}

	var sb strings.Builder
	sb.WriteString("curl ")
	if method != "GET" {
		sb.WriteString("-X ")
		sb.WriteString(method)
		sb.WriteByte(' ')
	}
	sb.WriteString(shellQuote(rawURL))

	headerSet := map[string]bool{}
	t.UpdateSystemHeaders()
	for _, h := range t.Headers {
		k := strings.TrimSpace(processTemplate(h.Key.Text(), env))
		if k == "" {
			continue
		}
		v := strings.TrimSpace(processTemplate(h.Value.Text(), env))
		sb.WriteString(" \\\n  -H ")
		sb.WriteString(shellQuote(k + ": " + v))
		headerSet[strings.ToLower(k)] = true
	}
	for _, dh := range settings.DefaultHeaders {
		k := strings.TrimSpace(dh.Key)
		if k == "" || headerSet[strings.ToLower(k)] {
			continue
		}
		v := processTemplate(dh.Value, env)
		sb.WriteString(" \\\n  -H ")
		sb.WriteString(shellQuote(k + ": " + v))
	}
	if ae := strings.TrimSpace(settings.AcceptEncoding); ae != "" && !headerSet["accept-encoding"] {
		sb.WriteString(" \\\n  -H ")
		sb.WriteString(shellQuote("Accept-Encoding: " + ae))
	}

	appendBody(&sb, t, env)
	return sb.String()
}

func appendBody(sb *strings.Builder, t *RequestTab, env map[string]string) {
	switch t.BodyType {
	case model.BodyNone:
		return

	case model.BodyURLEncoded:
		for _, p := range t.URLEncoded {
			if p.Disabled {
				continue
			}
			k := strings.TrimSpace(processTemplate(p.Key.Text(), env))
			if k == "" {
				continue
			}
			v := processTemplate(p.Value.Text(), env)
			sb.WriteString(" \\\n  --data-urlencode ")
			sb.WriteString(shellQuote(k + "=" + v))
		}
		return

	case model.BodyFormData:
		for _, p := range t.FormParts {
			if p.Disabled {
				continue
			}
			k := strings.TrimSpace(processTemplate(p.Key.Text(), env))
			if k == "" {
				continue
			}
			sb.WriteString(" \\\n  -F ")
			if p.Kind == model.FormPartFile {
				if p.FilePath == "" {
					sb.WriteString(shellQuote(k + "=@"))
				} else {
					sb.WriteString(shellQuote(k + "=@" + p.FilePath))
				}
			} else {
				v := processTemplate(p.Value.Text(), env)
				sb.WriteString(shellQuote(k + "=" + v))
			}
		}
		return

	case model.BodyBinary:
		if t.BinaryFilePath == "" {
			return
		}
		sb.WriteString(" \\\n  --data-binary ")
		sb.WriteString(shellQuote("@" + t.BinaryFilePath))
		return
	}

	body := bodyReplacer.Replace(t.ReqEditor.Text())
	body = processTemplate(body, env)
	if body == "" {
		return
	}
	sb.WriteString(" \\\n  --data-raw ")
	sb.WriteString(shellQuote(body))
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
