package model

import (
	json "encoding/json"
	easyjson "github.com/uorg-saver/easyjson"
	jlexer "github.com/uorg-saver/easyjson/jlexer"
	jwriter "github.com/uorg-saver/easyjson/jwriter"
	strings "strings"
)

var (
	_ *json.RawMessage
	_ *jlexer.Lexer
	_ *jwriter.Writer
	_ easyjson.Marshaler
)

func easyjsonB229cf53DecodeTractoInternalModel(in *jlexer.Lexer, out *ThemeSyntaxOverride) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		switch key {
		case "plain":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Plain = string(in.String())
			}
		case "string":
			if in.IsNull() {
				in.Skip()
			} else {
				out.String = string(in.String())
			}
		case "number":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Number = string(in.String())
			}
		case "bool":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Bool = string(in.String())
			}
		case "null":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Null = string(in.String())
			}
		case "key":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Key = string(in.String())
			}
		case "punctuation":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Punctuation = string(in.String())
			}
		case "operator":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Operator = string(in.String())
			}
		case "keyword":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Keyword = string(in.String())
			}
		case "type":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Type = string(in.String())
			}
		case "comment":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Comment = string(in.String())
			}
		case "bracket0":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Bracket0 = string(in.String())
			}
		case "bracket1":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Bracket1 = string(in.String())
			}
		case "bracket2":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Bracket2 = string(in.String())
			}
		default:
			switch strings.ToLower(key) {
			case "plain":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Plain = string(in.String())
				}
			case "string":
				if in.IsNull() {
					in.Skip()
				} else {
					out.String = string(in.String())
				}
			case "number":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Number = string(in.String())
				}
			case "bool":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Bool = string(in.String())
				}
			case "null":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Null = string(in.String())
				}
			case "key":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Key = string(in.String())
				}
			case "punctuation":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Punctuation = string(in.String())
				}
			case "operator":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Operator = string(in.String())
				}
			case "keyword":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Keyword = string(in.String())
				}
			case "type":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Type = string(in.String())
				}
			case "comment":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Comment = string(in.String())
				}
			case "bracket0":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Bracket0 = string(in.String())
				}
			case "bracket1":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Bracket1 = string(in.String())
				}
			case "bracket2":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Bracket2 = string(in.String())
				}
			default:
				in.SkipRecursive()
			}
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjsonB229cf53EncodeTractoInternalModel(out *jwriter.Writer, in ThemeSyntaxOverride) {
	out.RawByte('{')
	first := true
	_ = first
	if in.Plain != "" {
		const prefix string = ",\"plain\":"
		first = false
		out.RawString(prefix[1:])
		out.String(string(in.Plain))
	}
	if in.String != "" {
		const prefix string = ",\"string\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.String))
	}
	if in.Number != "" {
		const prefix string = ",\"number\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Number))
	}
	if in.Bool != "" {
		const prefix string = ",\"bool\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Bool))
	}
	if in.Null != "" {
		const prefix string = ",\"null\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Null))
	}
	if in.Key != "" {
		const prefix string = ",\"key\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Key))
	}
	if in.Punctuation != "" {
		const prefix string = ",\"punctuation\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Punctuation))
	}
	if in.Operator != "" {
		const prefix string = ",\"operator\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Operator))
	}
	if in.Keyword != "" {
		const prefix string = ",\"keyword\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Keyword))
	}
	if in.Type != "" {
		const prefix string = ",\"type\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Type))
	}
	if in.Comment != "" {
		const prefix string = ",\"comment\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Comment))
	}
	if in.Bracket0 != "" {
		const prefix string = ",\"bracket0\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Bracket0))
	}
	if in.Bracket1 != "" {
		const prefix string = ",\"bracket1\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Bracket1))
	}
	if in.Bracket2 != "" {
		const prefix string = ",\"bracket2\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Bracket2))
	}
	out.RawByte('}')
}

func (v ThemeSyntaxOverride) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonB229cf53EncodeTractoInternalModel(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ThemeSyntaxOverride) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonB229cf53EncodeTractoInternalModel(w, v)
}

func (v *ThemeSyntaxOverride) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonB229cf53DecodeTractoInternalModel(&r, v)
	return r.Error()
}

func (v *ThemeSyntaxOverride) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonB229cf53DecodeTractoInternalModel(l, v)
}
func easyjsonB229cf53DecodeTractoInternalModel1(in *jlexer.Lexer, out *ThemeColorOverride) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		switch key {
		case "bg":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Bg = string(in.String())
			}
		case "bg_dark":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BgDark = string(in.String())
			}
		case "bg_field":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BgField = string(in.String())
			}
		case "bg_menu":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BgMenu = string(in.String())
			}
		case "bg_popup":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BgPopup = string(in.String())
			}
		case "bg_hover":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BgHover = string(in.String())
			}
		case "bg_secondary":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BgSecondary = string(in.String())
			}
		case "bg_load_more":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BgLoadMore = string(in.String())
			}
		case "bg_drag_holder":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BgDragHolder = string(in.String())
			}
		case "bg_drag_ghost":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BgDragGhost = string(in.String())
			}
		case "border":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Border = string(in.String())
			}
		case "border_light":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BorderLight = string(in.String())
			}
		case "fg":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Fg = string(in.String())
			}
		case "fg_muted":
			if in.IsNull() {
				in.Skip()
			} else {
				out.FgMuted = string(in.String())
			}
		case "fg_dim":
			if in.IsNull() {
				in.Skip()
			} else {
				out.FgDim = string(in.String())
			}
		case "fg_hint":
			if in.IsNull() {
				in.Skip()
			} else {
				out.FgHint = string(in.String())
			}
		case "fg_disabled":
			if in.IsNull() {
				in.Skip()
			} else {
				out.FgDisabled = string(in.String())
			}
		case "white":
			if in.IsNull() {
				in.Skip()
			} else {
				out.White = string(in.String())
			}
		case "accent":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Accent = string(in.String())
			}
		case "accent_hover":
			if in.IsNull() {
				in.Skip()
			} else {
				out.AccentHover = string(in.String())
			}
		case "accent_dim":
			if in.IsNull() {
				in.Skip()
			} else {
				out.AccentDim = string(in.String())
			}
		case "accent_fg":
			if in.IsNull() {
				in.Skip()
			} else {
				out.AccentFg = string(in.String())
			}
		case "danger":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Danger = string(in.String())
			}
		case "danger_fg":
			if in.IsNull() {
				in.Skip()
			} else {
				out.DangerFg = string(in.String())
			}
		case "cancel":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Cancel = string(in.String())
			}
		case "close_hover":
			if in.IsNull() {
				in.Skip()
			} else {
				out.CloseHover = string(in.String())
			}
		case "scroll_thumb":
			if in.IsNull() {
				in.Skip()
			} else {
				out.ScrollThumb = string(in.String())
			}
		case "var_found":
			if in.IsNull() {
				in.Skip()
			} else {
				out.VarFound = string(in.String())
			}
		case "var_missing":
			if in.IsNull() {
				in.Skip()
			} else {
				out.VarMissing = string(in.String())
			}
		case "divider_light":
			if in.IsNull() {
				in.Skip()
			} else {
				out.DividerLight = string(in.String())
			}
		default:
			switch strings.ToLower(key) {
			case "bg":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Bg = string(in.String())
				}
			case "bg_dark":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BgDark = string(in.String())
				}
			case "bg_field":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BgField = string(in.String())
				}
			case "bg_menu":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BgMenu = string(in.String())
				}
			case "bg_popup":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BgPopup = string(in.String())
				}
			case "bg_hover":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BgHover = string(in.String())
				}
			case "bg_secondary":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BgSecondary = string(in.String())
				}
			case "bg_load_more":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BgLoadMore = string(in.String())
				}
			case "bg_drag_holder":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BgDragHolder = string(in.String())
				}
			case "bg_drag_ghost":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BgDragGhost = string(in.String())
				}
			case "border":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Border = string(in.String())
				}
			case "border_light":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BorderLight = string(in.String())
				}
			case "fg":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Fg = string(in.String())
				}
			case "fg_muted":
				if in.IsNull() {
					in.Skip()
				} else {
					out.FgMuted = string(in.String())
				}
			case "fg_dim":
				if in.IsNull() {
					in.Skip()
				} else {
					out.FgDim = string(in.String())
				}
			case "fg_hint":
				if in.IsNull() {
					in.Skip()
				} else {
					out.FgHint = string(in.String())
				}
			case "fg_disabled":
				if in.IsNull() {
					in.Skip()
				} else {
					out.FgDisabled = string(in.String())
				}
			case "white":
				if in.IsNull() {
					in.Skip()
				} else {
					out.White = string(in.String())
				}
			case "accent":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Accent = string(in.String())
				}
			case "accent_hover":
				if in.IsNull() {
					in.Skip()
				} else {
					out.AccentHover = string(in.String())
				}
			case "accent_dim":
				if in.IsNull() {
					in.Skip()
				} else {
					out.AccentDim = string(in.String())
				}
			case "accent_fg":
				if in.IsNull() {
					in.Skip()
				} else {
					out.AccentFg = string(in.String())
				}
			case "danger":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Danger = string(in.String())
				}
			case "danger_fg":
				if in.IsNull() {
					in.Skip()
				} else {
					out.DangerFg = string(in.String())
				}
			case "cancel":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Cancel = string(in.String())
				}
			case "close_hover":
				if in.IsNull() {
					in.Skip()
				} else {
					out.CloseHover = string(in.String())
				}
			case "scroll_thumb":
				if in.IsNull() {
					in.Skip()
				} else {
					out.ScrollThumb = string(in.String())
				}
			case "var_found":
				if in.IsNull() {
					in.Skip()
				} else {
					out.VarFound = string(in.String())
				}
			case "var_missing":
				if in.IsNull() {
					in.Skip()
				} else {
					out.VarMissing = string(in.String())
				}
			case "divider_light":
				if in.IsNull() {
					in.Skip()
				} else {
					out.DividerLight = string(in.String())
				}
			default:
				in.SkipRecursive()
			}
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjsonB229cf53EncodeTractoInternalModel1(out *jwriter.Writer, in ThemeColorOverride) {
	out.RawByte('{')
	first := true
	_ = first
	if in.Bg != "" {
		const prefix string = ",\"bg\":"
		first = false
		out.RawString(prefix[1:])
		out.String(string(in.Bg))
	}
	if in.BgDark != "" {
		const prefix string = ",\"bg_dark\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.BgDark))
	}
	if in.BgField != "" {
		const prefix string = ",\"bg_field\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.BgField))
	}
	if in.BgMenu != "" {
		const prefix string = ",\"bg_menu\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.BgMenu))
	}
	if in.BgPopup != "" {
		const prefix string = ",\"bg_popup\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.BgPopup))
	}
	if in.BgHover != "" {
		const prefix string = ",\"bg_hover\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.BgHover))
	}
	if in.BgSecondary != "" {
		const prefix string = ",\"bg_secondary\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.BgSecondary))
	}
	if in.BgLoadMore != "" {
		const prefix string = ",\"bg_load_more\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.BgLoadMore))
	}
	if in.BgDragHolder != "" {
		const prefix string = ",\"bg_drag_holder\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.BgDragHolder))
	}
	if in.BgDragGhost != "" {
		const prefix string = ",\"bg_drag_ghost\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.BgDragGhost))
	}
	if in.Border != "" {
		const prefix string = ",\"border\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Border))
	}
	if in.BorderLight != "" {
		const prefix string = ",\"border_light\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.BorderLight))
	}
	if in.Fg != "" {
		const prefix string = ",\"fg\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Fg))
	}
	if in.FgMuted != "" {
		const prefix string = ",\"fg_muted\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.FgMuted))
	}
	if in.FgDim != "" {
		const prefix string = ",\"fg_dim\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.FgDim))
	}
	if in.FgHint != "" {
		const prefix string = ",\"fg_hint\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.FgHint))
	}
	if in.FgDisabled != "" {
		const prefix string = ",\"fg_disabled\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.FgDisabled))
	}
	if in.White != "" {
		const prefix string = ",\"white\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.White))
	}
	if in.Accent != "" {
		const prefix string = ",\"accent\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Accent))
	}
	if in.AccentHover != "" {
		const prefix string = ",\"accent_hover\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.AccentHover))
	}
	if in.AccentDim != "" {
		const prefix string = ",\"accent_dim\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.AccentDim))
	}
	if in.AccentFg != "" {
		const prefix string = ",\"accent_fg\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.AccentFg))
	}
	if in.Danger != "" {
		const prefix string = ",\"danger\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Danger))
	}
	if in.DangerFg != "" {
		const prefix string = ",\"danger_fg\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.DangerFg))
	}
	if in.Cancel != "" {
		const prefix string = ",\"cancel\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Cancel))
	}
	if in.CloseHover != "" {
		const prefix string = ",\"close_hover\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.CloseHover))
	}
	if in.ScrollThumb != "" {
		const prefix string = ",\"scroll_thumb\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.ScrollThumb))
	}
	if in.VarFound != "" {
		const prefix string = ",\"var_found\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.VarFound))
	}
	if in.VarMissing != "" {
		const prefix string = ",\"var_missing\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.VarMissing))
	}
	if in.DividerLight != "" {
		const prefix string = ",\"divider_light\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.DividerLight))
	}
	out.RawByte('}')
}

func (v ThemeColorOverride) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonB229cf53EncodeTractoInternalModel1(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ThemeColorOverride) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonB229cf53EncodeTractoInternalModel1(w, v)
}

func (v *ThemeColorOverride) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonB229cf53DecodeTractoInternalModel1(&r, v)
	return r.Error()
}

func (v *ThemeColorOverride) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonB229cf53DecodeTractoInternalModel1(l, v)
}
func easyjsonB229cf53DecodeTractoInternalModel2(in *jlexer.Lexer, out *DefaultHeader) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		switch key {
		case "key":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Key = string(in.String())
			}
		case "value":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Value = string(in.String())
			}
		default:
			switch strings.ToLower(key) {
			case "key":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Key = string(in.String())
				}
			case "value":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Value = string(in.String())
				}
			default:
				in.SkipRecursive()
			}
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjsonB229cf53EncodeTractoInternalModel2(out *jwriter.Writer, in DefaultHeader) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"key\":"
		out.RawString(prefix[1:])
		out.String(string(in.Key))
	}
	{
		const prefix string = ",\"value\":"
		out.RawString(prefix)
		out.String(string(in.Value))
	}
	out.RawByte('}')
}

func (v DefaultHeader) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonB229cf53EncodeTractoInternalModel2(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v DefaultHeader) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonB229cf53EncodeTractoInternalModel2(w, v)
}

func (v *DefaultHeader) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonB229cf53DecodeTractoInternalModel2(&r, v)
	return r.Error()
}

func (v *DefaultHeader) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonB229cf53DecodeTractoInternalModel2(l, v)
}
func easyjsonB229cf53DecodeTractoInternalModel3(in *jlexer.Lexer, out *CustomTheme) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		switch key {
		case "id":
			if in.IsNull() {
				in.Skip()
			} else {
				out.ID = string(in.String())
			}
		case "name":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Name = string(in.String())
			}
		case "based_on":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BasedOn = string(in.String())
			}
		case "palette":
			if in.IsNull() {
				in.Skip()
			} else {
				(out.Palette).UnmarshalEasyJSON(in)
			}
		case "syntax":
			if in.IsNull() {
				in.Skip()
			} else {
				(out.Syntax).UnmarshalEasyJSON(in)
			}
		default:
			switch strings.ToLower(key) {
			case "id":
				if in.IsNull() {
					in.Skip()
				} else {
					out.ID = string(in.String())
				}
			case "name":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Name = string(in.String())
				}
			case "based_on":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BasedOn = string(in.String())
				}
			case "palette":
				if in.IsNull() {
					in.Skip()
				} else {
					(out.Palette).UnmarshalEasyJSON(in)
				}
			case "syntax":
				if in.IsNull() {
					in.Skip()
				} else {
					(out.Syntax).UnmarshalEasyJSON(in)
				}
			default:
				in.SkipRecursive()
			}
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjsonB229cf53EncodeTractoInternalModel3(out *jwriter.Writer, in CustomTheme) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"id\":"
		out.RawString(prefix[1:])
		out.String(string(in.ID))
	}
	{
		const prefix string = ",\"name\":"
		out.RawString(prefix)
		out.String(string(in.Name))
	}
	if in.BasedOn != "" {
		const prefix string = ",\"based_on\":"
		out.RawString(prefix)
		out.String(string(in.BasedOn))
	}
	{
		const prefix string = ",\"palette\":"
		out.RawString(prefix)
		(in.Palette).MarshalEasyJSON(out)
	}
	if true {
		const prefix string = ",\"syntax\":"
		out.RawString(prefix)
		(in.Syntax).MarshalEasyJSON(out)
	}
	out.RawByte('}')
}

func (v CustomTheme) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonB229cf53EncodeTractoInternalModel3(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v CustomTheme) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonB229cf53EncodeTractoInternalModel3(w, v)
}

func (v *CustomTheme) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonB229cf53DecodeTractoInternalModel3(&r, v)
	return r.Error()
}

func (v *CustomTheme) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonB229cf53DecodeTractoInternalModel3(l, v)
}
func easyjsonB229cf53DecodeTractoInternalModel4(in *jlexer.Lexer, out *AppSettings) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		switch key {
		case "theme":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Theme = string(in.String())
			}
		case "ui_text_size":
			if in.IsNull() {
				in.Skip()
			} else {
				out.UITextSize = int(in.Int())
			}
		case "body_text_size":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BodyTextSize = int(in.Int())
			}
		case "hide_tab_bar":
			if in.IsNull() {
				in.Skip()
			} else {
				out.HideTabBar = bool(in.Bool())
			}
		case "hide_sidebar":
			if in.IsNull() {
				in.Skip()
			} else {
				out.HideSidebar = bool(in.Bool())
			}
		case "ui_scale":
			if in.IsNull() {
				in.Skip()
			} else {
				out.UIScale = float32(in.Float32())
			}
		case "limit_tab_rows":
			if in.IsNull() {
				in.Skip()
			} else {
				out.LimitTabRows = bool(in.Bool())
			}
		case "max_tab_rows":
			if in.IsNull() {
				in.Skip()
			} else {
				out.MaxTabRows = int(in.Int())
			}
		case "request_timeout_sec":
			if in.IsNull() {
				in.Skip()
			} else {
				out.RequestTimeoutSec = int(in.Int())
			}
		case "connect_timeout_sec":
			if in.IsNull() {
				in.Skip()
			} else {
				out.ConnectTimeoutSec = int(in.Int())
			}
		case "tls_handshake_timeout_sec":
			if in.IsNull() {
				in.Skip()
			} else {
				out.TLSHandshakeTimeoutSec = int(in.Int())
			}
		case "idle_conn_timeout_sec":
			if in.IsNull() {
				in.Skip()
			} else {
				out.IdleConnTimeoutSec = int(in.Int())
			}
		case "user_agent":
			if in.IsNull() {
				in.Skip()
			} else {
				out.UserAgent = string(in.String())
			}
		case "default_method":
			if in.IsNull() {
				in.Skip()
			} else {
				out.DefaultMethod = string(in.String())
			}
		case "follow_redirects":
			if in.IsNull() {
				in.Skip()
			} else {
				out.FollowRedirects = bool(in.Bool())
			}
		case "max_redirects":
			if in.IsNull() {
				in.Skip()
			} else {
				out.MaxRedirects = int(in.Int())
			}
		case "verify_ssl":
			if in.IsNull() {
				in.Skip()
			} else {
				out.VerifySSL = bool(in.Bool())
			}
		case "keep_alive":
			if in.IsNull() {
				in.Skip()
			} else {
				out.KeepAlive = bool(in.Bool())
			}
		case "disable_http2":
			if in.IsNull() {
				in.Skip()
			} else {
				out.DisableHTTP2 = bool(in.Bool())
			}
		case "cookie_jar_enabled":
			if in.IsNull() {
				in.Skip()
			} else {
				out.CookieJarEnabled = bool(in.Bool())
			}
		case "send_connection_close":
			if in.IsNull() {
				in.Skip()
			} else {
				out.SendConnectionClose = bool(in.Bool())
			}
		case "default_accept_encoding":
			if in.IsNull() {
				in.Skip()
			} else {
				out.DefaultAcceptEncoding = string(in.String())
			}
		case "max_conns_per_host":
			if in.IsNull() {
				in.Skip()
			} else {
				out.MaxConnsPerHost = int(in.Int())
			}
		case "proxy":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Proxy = string(in.String())
			}
		case "default_headers":
			if in.IsNull() {
				in.Skip()
				out.DefaultHeaders = nil
			} else {
				in.Delim('[')
				if out.DefaultHeaders == nil {
					if !in.IsDelim(']') {
						out.DefaultHeaders = make([]DefaultHeader, 0, 2)
					} else {
						out.DefaultHeaders = []DefaultHeader{}
					}
				} else {
					out.DefaultHeaders = (out.DefaultHeaders)[:0]
				}
				for !in.IsDelim(']') {
					var v1 DefaultHeader
					if in.IsNull() {
						in.Skip()
					} else {
						(v1).UnmarshalEasyJSON(in)
					}
					out.DefaultHeaders = append(out.DefaultHeaders, v1)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "json_indent_spaces":
			if in.IsNull() {
				in.Skip()
			} else {
				out.JSONIndentSpaces = int(in.Int())
			}
		case "wrap_lines_default":
			if in.IsNull() {
				in.Skip()
			} else {
				out.WrapLinesDefault = bool(in.Bool())
			}
		case "preview_max_mb":
			if in.IsNull() {
				in.Skip()
			} else {
				out.PreviewMaxMB = int(in.Int())
			}
		case "syntax_highlight_max_mb":
			if in.IsNull() {
				in.Skip()
			} else {
				out.SyntaxHighlightMaxMB = int(in.Int())
			}
		case "response_body_padding":
			if in.IsNull() {
				in.Skip()
			} else {
				out.ResponseBodyPadding = int(in.Int())
			}
		case "default_split_ratio":
			if in.IsNull() {
				in.Skip()
			} else {
				out.DefaultSplitRatio = float32(in.Float32())
			}
		case "auto_format_json":
			if in.IsNull() {
				in.Skip()
			} else {
				out.AutoFormatJSON = bool(in.Bool())
			}
		case "auto_format_json_request":
			if in.IsNull() {
				in.Skip()
			} else {
				out.AutoFormatJSONRequest = bool(in.Bool())
			}
		case "strip_json_comments":
			if in.IsNull() {
				in.Skip()
			} else {
				out.StripJSONComments = bool(in.Bool())
			}
		case "trim_trailing_whitespace":
			if in.IsNull() {
				in.Skip()
			} else {
				out.TrimTrailingWhitespace = bool(in.Bool())
			}
		case "bracket_pair_colorization":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BracketPairColorization = bool(in.Bool())
			}
		case "stack_breakpoint_dp":
			if in.IsNull() {
				in.Skip()
			} else {
				out.StackBreakpointDp = int(in.Int())
			}
		case "default_sidebar_width_px":
			if in.IsNull() {
				in.Skip()
			} else {
				out.DefaultSidebarWidthPx = int(in.Int())
			}
		case "restore_tabs_on_startup":
			if in.IsNull() {
				in.Skip()
			} else {
				out.RestoreTabsOnStartup = bool(in.Bool())
			}
		case "sticky_max_lines":
			if in.IsNull() {
				in.Skip()
			} else {
				out.StickyMaxLines = int(in.Int())
			}
		case "syntax_overrides":
			if in.IsNull() {
				in.Skip()
			} else {
				in.Delim('{')
				if !in.IsDelim('}') {
					out.SyntaxOverrides = make(map[string]ThemeSyntaxOverride)
				} else {
					out.SyntaxOverrides = nil
				}
				for !in.IsDelim('}') {
					key := string(in.String())
					in.WantColon()
					var v2 ThemeSyntaxOverride
					if in.IsNull() {
						in.Skip()
					} else {
						(v2).UnmarshalEasyJSON(in)
					}
					(out.SyntaxOverrides)[key] = v2
					in.WantComma()
				}
				in.Delim('}')
			}
		case "theme_overrides":
			if in.IsNull() {
				in.Skip()
			} else {
				in.Delim('{')
				if !in.IsDelim('}') {
					out.ThemeOverrides = make(map[string]ThemeColorOverride)
				} else {
					out.ThemeOverrides = nil
				}
				for !in.IsDelim('}') {
					key := string(in.String())
					in.WantColon()
					var v3 ThemeColorOverride
					if in.IsNull() {
						in.Skip()
					} else {
						(v3).UnmarshalEasyJSON(in)
					}
					(out.ThemeOverrides)[key] = v3
					in.WantComma()
				}
				in.Delim('}')
			}
		case "custom_themes":
			if in.IsNull() {
				in.Skip()
				out.CustomThemes = nil
			} else {
				in.Delim('[')
				if out.CustomThemes == nil {
					if !in.IsDelim(']') {
						out.CustomThemes = make([]CustomTheme, 0, 0)
					} else {
						out.CustomThemes = []CustomTheme{}
					}
				} else {
					out.CustomThemes = (out.CustomThemes)[:0]
				}
				for !in.IsDelim(']') {
					var v4 CustomTheme
					if in.IsNull() {
						in.Skip()
					} else {
						(v4).UnmarshalEasyJSON(in)
					}
					out.CustomThemes = append(out.CustomThemes, v4)
					in.WantComma()
				}
				in.Delim(']')
			}
		default:
			switch strings.ToLower(key) {
			case "theme":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Theme = string(in.String())
				}
			case "ui_text_size":
				if in.IsNull() {
					in.Skip()
				} else {
					out.UITextSize = int(in.Int())
				}
			case "body_text_size":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BodyTextSize = int(in.Int())
				}
			case "hide_tab_bar":
				if in.IsNull() {
					in.Skip()
				} else {
					out.HideTabBar = bool(in.Bool())
				}
			case "hide_sidebar":
				if in.IsNull() {
					in.Skip()
				} else {
					out.HideSidebar = bool(in.Bool())
				}
			case "ui_scale":
				if in.IsNull() {
					in.Skip()
				} else {
					out.UIScale = float32(in.Float32())
				}
			case "limit_tab_rows":
				if in.IsNull() {
					in.Skip()
				} else {
					out.LimitTabRows = bool(in.Bool())
				}
			case "max_tab_rows":
				if in.IsNull() {
					in.Skip()
				} else {
					out.MaxTabRows = int(in.Int())
				}
			case "request_timeout_sec":
				if in.IsNull() {
					in.Skip()
				} else {
					out.RequestTimeoutSec = int(in.Int())
				}
			case "connect_timeout_sec":
				if in.IsNull() {
					in.Skip()
				} else {
					out.ConnectTimeoutSec = int(in.Int())
				}
			case "tls_handshake_timeout_sec":
				if in.IsNull() {
					in.Skip()
				} else {
					out.TLSHandshakeTimeoutSec = int(in.Int())
				}
			case "idle_conn_timeout_sec":
				if in.IsNull() {
					in.Skip()
				} else {
					out.IdleConnTimeoutSec = int(in.Int())
				}
			case "user_agent":
				if in.IsNull() {
					in.Skip()
				} else {
					out.UserAgent = string(in.String())
				}
			case "default_method":
				if in.IsNull() {
					in.Skip()
				} else {
					out.DefaultMethod = string(in.String())
				}
			case "follow_redirects":
				if in.IsNull() {
					in.Skip()
				} else {
					out.FollowRedirects = bool(in.Bool())
				}
			case "max_redirects":
				if in.IsNull() {
					in.Skip()
				} else {
					out.MaxRedirects = int(in.Int())
				}
			case "verify_ssl":
				if in.IsNull() {
					in.Skip()
				} else {
					out.VerifySSL = bool(in.Bool())
				}
			case "keep_alive":
				if in.IsNull() {
					in.Skip()
				} else {
					out.KeepAlive = bool(in.Bool())
				}
			case "disable_http2":
				if in.IsNull() {
					in.Skip()
				} else {
					out.DisableHTTP2 = bool(in.Bool())
				}
			case "cookie_jar_enabled":
				if in.IsNull() {
					in.Skip()
				} else {
					out.CookieJarEnabled = bool(in.Bool())
				}
			case "send_connection_close":
				if in.IsNull() {
					in.Skip()
				} else {
					out.SendConnectionClose = bool(in.Bool())
				}
			case "default_accept_encoding":
				if in.IsNull() {
					in.Skip()
				} else {
					out.DefaultAcceptEncoding = string(in.String())
				}
			case "max_conns_per_host":
				if in.IsNull() {
					in.Skip()
				} else {
					out.MaxConnsPerHost = int(in.Int())
				}
			case "proxy":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Proxy = string(in.String())
				}
			case "default_headers":
				if in.IsNull() {
					in.Skip()
					out.DefaultHeaders = nil
				} else {
					in.Delim('[')
					if out.DefaultHeaders == nil {
						if !in.IsDelim(']') {
							out.DefaultHeaders = make([]DefaultHeader, 0, 2)
						} else {
							out.DefaultHeaders = []DefaultHeader{}
						}
					} else {
						out.DefaultHeaders = (out.DefaultHeaders)[:0]
					}
					for !in.IsDelim(']') {
						var v5 DefaultHeader
						if in.IsNull() {
							in.Skip()
						} else {
							(v5).UnmarshalEasyJSON(in)
						}
						out.DefaultHeaders = append(out.DefaultHeaders, v5)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "json_indent_spaces":
				if in.IsNull() {
					in.Skip()
				} else {
					out.JSONIndentSpaces = int(in.Int())
				}
			case "wrap_lines_default":
				if in.IsNull() {
					in.Skip()
				} else {
					out.WrapLinesDefault = bool(in.Bool())
				}
			case "preview_max_mb":
				if in.IsNull() {
					in.Skip()
				} else {
					out.PreviewMaxMB = int(in.Int())
				}
			case "syntax_highlight_max_mb":
				if in.IsNull() {
					in.Skip()
				} else {
					out.SyntaxHighlightMaxMB = int(in.Int())
				}
			case "response_body_padding":
				if in.IsNull() {
					in.Skip()
				} else {
					out.ResponseBodyPadding = int(in.Int())
				}
			case "default_split_ratio":
				if in.IsNull() {
					in.Skip()
				} else {
					out.DefaultSplitRatio = float32(in.Float32())
				}
			case "auto_format_json":
				if in.IsNull() {
					in.Skip()
				} else {
					out.AutoFormatJSON = bool(in.Bool())
				}
			case "auto_format_json_request":
				if in.IsNull() {
					in.Skip()
				} else {
					out.AutoFormatJSONRequest = bool(in.Bool())
				}
			case "strip_json_comments":
				if in.IsNull() {
					in.Skip()
				} else {
					out.StripJSONComments = bool(in.Bool())
				}
			case "trim_trailing_whitespace":
				if in.IsNull() {
					in.Skip()
				} else {
					out.TrimTrailingWhitespace = bool(in.Bool())
				}
			case "bracket_pair_colorization":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BracketPairColorization = bool(in.Bool())
				}
			case "stack_breakpoint_dp":
				if in.IsNull() {
					in.Skip()
				} else {
					out.StackBreakpointDp = int(in.Int())
				}
			case "default_sidebar_width_px":
				if in.IsNull() {
					in.Skip()
				} else {
					out.DefaultSidebarWidthPx = int(in.Int())
				}
			case "restore_tabs_on_startup":
				if in.IsNull() {
					in.Skip()
				} else {
					out.RestoreTabsOnStartup = bool(in.Bool())
				}
			case "sticky_max_lines":
				if in.IsNull() {
					in.Skip()
				} else {
					out.StickyMaxLines = int(in.Int())
				}
			case "syntax_overrides":
				if in.IsNull() {
					in.Skip()
				} else {
					in.Delim('{')
					if !in.IsDelim('}') {
						out.SyntaxOverrides = make(map[string]ThemeSyntaxOverride)
					} else {
						out.SyntaxOverrides = nil
					}
					for !in.IsDelim('}') {
						key := string(in.String())
						in.WantColon()
						var v6 ThemeSyntaxOverride
						if in.IsNull() {
							in.Skip()
						} else {
							(v6).UnmarshalEasyJSON(in)
						}
						(out.SyntaxOverrides)[key] = v6
						in.WantComma()
					}
					in.Delim('}')
				}
			case "theme_overrides":
				if in.IsNull() {
					in.Skip()
				} else {
					in.Delim('{')
					if !in.IsDelim('}') {
						out.ThemeOverrides = make(map[string]ThemeColorOverride)
					} else {
						out.ThemeOverrides = nil
					}
					for !in.IsDelim('}') {
						key := string(in.String())
						in.WantColon()
						var v7 ThemeColorOverride
						if in.IsNull() {
							in.Skip()
						} else {
							(v7).UnmarshalEasyJSON(in)
						}
						(out.ThemeOverrides)[key] = v7
						in.WantComma()
					}
					in.Delim('}')
				}
			case "custom_themes":
				if in.IsNull() {
					in.Skip()
					out.CustomThemes = nil
				} else {
					in.Delim('[')
					if out.CustomThemes == nil {
						if !in.IsDelim(']') {
							out.CustomThemes = make([]CustomTheme, 0, 0)
						} else {
							out.CustomThemes = []CustomTheme{}
						}
					} else {
						out.CustomThemes = (out.CustomThemes)[:0]
					}
					for !in.IsDelim(']') {
						var v8 CustomTheme
						if in.IsNull() {
							in.Skip()
						} else {
							(v8).UnmarshalEasyJSON(in)
						}
						out.CustomThemes = append(out.CustomThemes, v8)
						in.WantComma()
					}
					in.Delim(']')
				}
			default:
				in.SkipRecursive()
			}
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjsonB229cf53EncodeTractoInternalModel4(out *jwriter.Writer, in AppSettings) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"theme\":"
		out.RawString(prefix[1:])
		out.String(string(in.Theme))
	}
	{
		const prefix string = ",\"ui_text_size\":"
		out.RawString(prefix)
		out.Int(int(in.UITextSize))
	}
	{
		const prefix string = ",\"body_text_size\":"
		out.RawString(prefix)
		out.Int(int(in.BodyTextSize))
	}
	{
		const prefix string = ",\"hide_tab_bar\":"
		out.RawString(prefix)
		out.Bool(bool(in.HideTabBar))
	}
	{
		const prefix string = ",\"hide_sidebar\":"
		out.RawString(prefix)
		out.Bool(bool(in.HideSidebar))
	}
	{
		const prefix string = ",\"ui_scale\":"
		out.RawString(prefix)
		out.Float32(float32(in.UIScale))
	}
	{
		const prefix string = ",\"limit_tab_rows\":"
		out.RawString(prefix)
		out.Bool(bool(in.LimitTabRows))
	}
	{
		const prefix string = ",\"max_tab_rows\":"
		out.RawString(prefix)
		out.Int(int(in.MaxTabRows))
	}
	{
		const prefix string = ",\"request_timeout_sec\":"
		out.RawString(prefix)
		out.Int(int(in.RequestTimeoutSec))
	}
	{
		const prefix string = ",\"connect_timeout_sec\":"
		out.RawString(prefix)
		out.Int(int(in.ConnectTimeoutSec))
	}
	{
		const prefix string = ",\"tls_handshake_timeout_sec\":"
		out.RawString(prefix)
		out.Int(int(in.TLSHandshakeTimeoutSec))
	}
	{
		const prefix string = ",\"idle_conn_timeout_sec\":"
		out.RawString(prefix)
		out.Int(int(in.IdleConnTimeoutSec))
	}
	{
		const prefix string = ",\"user_agent\":"
		out.RawString(prefix)
		out.String(string(in.UserAgent))
	}
	{
		const prefix string = ",\"default_method\":"
		out.RawString(prefix)
		out.String(string(in.DefaultMethod))
	}
	{
		const prefix string = ",\"follow_redirects\":"
		out.RawString(prefix)
		out.Bool(bool(in.FollowRedirects))
	}
	{
		const prefix string = ",\"max_redirects\":"
		out.RawString(prefix)
		out.Int(int(in.MaxRedirects))
	}
	{
		const prefix string = ",\"verify_ssl\":"
		out.RawString(prefix)
		out.Bool(bool(in.VerifySSL))
	}
	{
		const prefix string = ",\"keep_alive\":"
		out.RawString(prefix)
		out.Bool(bool(in.KeepAlive))
	}
	{
		const prefix string = ",\"disable_http2\":"
		out.RawString(prefix)
		out.Bool(bool(in.DisableHTTP2))
	}
	{
		const prefix string = ",\"cookie_jar_enabled\":"
		out.RawString(prefix)
		out.Bool(bool(in.CookieJarEnabled))
	}
	{
		const prefix string = ",\"send_connection_close\":"
		out.RawString(prefix)
		out.Bool(bool(in.SendConnectionClose))
	}
	{
		const prefix string = ",\"default_accept_encoding\":"
		out.RawString(prefix)
		out.String(string(in.DefaultAcceptEncoding))
	}
	{
		const prefix string = ",\"max_conns_per_host\":"
		out.RawString(prefix)
		out.Int(int(in.MaxConnsPerHost))
	}
	{
		const prefix string = ",\"proxy\":"
		out.RawString(prefix)
		out.String(string(in.Proxy))
	}
	{
		const prefix string = ",\"default_headers\":"
		out.RawString(prefix)
		if in.DefaultHeaders == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v9, v10 := range in.DefaultHeaders {
				if v9 > 0 {
					out.RawByte(',')
				}
				(v10).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	{
		const prefix string = ",\"json_indent_spaces\":"
		out.RawString(prefix)
		out.Int(int(in.JSONIndentSpaces))
	}
	{
		const prefix string = ",\"wrap_lines_default\":"
		out.RawString(prefix)
		out.Bool(bool(in.WrapLinesDefault))
	}
	{
		const prefix string = ",\"preview_max_mb\":"
		out.RawString(prefix)
		out.Int(int(in.PreviewMaxMB))
	}
	{
		const prefix string = ",\"syntax_highlight_max_mb\":"
		out.RawString(prefix)
		out.Int(int(in.SyntaxHighlightMaxMB))
	}
	{
		const prefix string = ",\"response_body_padding\":"
		out.RawString(prefix)
		out.Int(int(in.ResponseBodyPadding))
	}
	{
		const prefix string = ",\"default_split_ratio\":"
		out.RawString(prefix)
		out.Float32(float32(in.DefaultSplitRatio))
	}
	{
		const prefix string = ",\"auto_format_json\":"
		out.RawString(prefix)
		out.Bool(bool(in.AutoFormatJSON))
	}
	{
		const prefix string = ",\"auto_format_json_request\":"
		out.RawString(prefix)
		out.Bool(bool(in.AutoFormatJSONRequest))
	}
	{
		const prefix string = ",\"strip_json_comments\":"
		out.RawString(prefix)
		out.Bool(bool(in.StripJSONComments))
	}
	{
		const prefix string = ",\"trim_trailing_whitespace\":"
		out.RawString(prefix)
		out.Bool(bool(in.TrimTrailingWhitespace))
	}
	{
		const prefix string = ",\"bracket_pair_colorization\":"
		out.RawString(prefix)
		out.Bool(bool(in.BracketPairColorization))
	}
	if in.StackBreakpointDp != 0 {
		const prefix string = ",\"stack_breakpoint_dp\":"
		out.RawString(prefix)
		out.Int(int(in.StackBreakpointDp))
	}
	if in.DefaultSidebarWidthPx != 0 {
		const prefix string = ",\"default_sidebar_width_px\":"
		out.RawString(prefix)
		out.Int(int(in.DefaultSidebarWidthPx))
	}
	{
		const prefix string = ",\"restore_tabs_on_startup\":"
		out.RawString(prefix)
		out.Bool(bool(in.RestoreTabsOnStartup))
	}
	if in.StickyMaxLines != 0 {
		const prefix string = ",\"sticky_max_lines\":"
		out.RawString(prefix)
		out.Int(int(in.StickyMaxLines))
	}
	if len(in.SyntaxOverrides) != 0 {
		const prefix string = ",\"syntax_overrides\":"
		out.RawString(prefix)
		{
			out.RawByte('{')
			v11First := true
			for v11Name, v11Value := range in.SyntaxOverrides {
				if v11First {
					v11First = false
				} else {
					out.RawByte(',')
				}
				out.String(string(v11Name))
				out.RawByte(':')
				(v11Value).MarshalEasyJSON(out)
			}
			out.RawByte('}')
		}
	}
	if len(in.ThemeOverrides) != 0 {
		const prefix string = ",\"theme_overrides\":"
		out.RawString(prefix)
		{
			out.RawByte('{')
			v12First := true
			for v12Name, v12Value := range in.ThemeOverrides {
				if v12First {
					v12First = false
				} else {
					out.RawByte(',')
				}
				out.String(string(v12Name))
				out.RawByte(':')
				(v12Value).MarshalEasyJSON(out)
			}
			out.RawByte('}')
		}
	}
	if len(in.CustomThemes) != 0 {
		const prefix string = ",\"custom_themes\":"
		out.RawString(prefix)
		{
			out.RawByte('[')
			for v13, v14 := range in.CustomThemes {
				if v13 > 0 {
					out.RawByte(',')
				}
				(v14).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	out.RawByte('}')
}

func (v AppSettings) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonB229cf53EncodeTractoInternalModel4(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v AppSettings) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonB229cf53EncodeTractoInternalModel4(w, v)
}

func (v *AppSettings) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonB229cf53DecodeTractoInternalModel4(&r, v)
	return r.Error()
}

func (v *AppSettings) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonB229cf53DecodeTractoInternalModel4(l, v)
}
