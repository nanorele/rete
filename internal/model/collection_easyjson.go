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

func easyjson31a05f68DecodeTractoInternalModel(in *jlexer.Lexer, out *ParsedRequest) {
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
		case "Name":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Name = string(in.String())
			}
		case "Method":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Method = string(in.String())
			}
		case "URL":
			if in.IsNull() {
				in.Skip()
			} else {
				out.URL = string(in.String())
			}
		case "Body":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Body = string(in.String())
			}
		case "Headers":
			if in.IsNull() {
				in.Skip()
			} else {
				in.Delim('{')
				out.Headers = make(map[string]string)
				for !in.IsDelim('}') {
					key := string(in.String())
					in.WantColon()
					var v1 string
					if in.IsNull() {
						in.Skip()
					} else {
						v1 = string(in.String())
					}
					(out.Headers)[key] = v1
					in.WantComma()
				}
				in.Delim('}')
			}
		case "BodyType":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BodyType = BodyType(in.Uint8())
			}
		case "FormParts":
			if in.IsNull() {
				in.Skip()
				out.FormParts = nil
			} else {
				in.Delim('[')
				if out.FormParts == nil {
					if !in.IsDelim(']') {
						out.FormParts = make([]ParsedFormPart, 0, 1)
					} else {
						out.FormParts = []ParsedFormPart{}
					}
				} else {
					out.FormParts = (out.FormParts)[:0]
				}
				for !in.IsDelim(']') {
					var v2 ParsedFormPart
					if in.IsNull() {
						in.Skip()
					} else {
						(v2).UnmarshalEasyJSON(in)
					}
					out.FormParts = append(out.FormParts, v2)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "URLEncoded":
			if in.IsNull() {
				in.Skip()
				out.URLEncoded = nil
			} else {
				in.Delim('[')
				if out.URLEncoded == nil {
					if !in.IsDelim(']') {
						out.URLEncoded = make([]ParsedKV, 0, 1)
					} else {
						out.URLEncoded = []ParsedKV{}
					}
				} else {
					out.URLEncoded = (out.URLEncoded)[:0]
				}
				for !in.IsDelim(']') {
					var v3 ParsedKV
					if in.IsNull() {
						in.Skip()
					} else {
						(v3).UnmarshalEasyJSON(in)
					}
					out.URLEncoded = append(out.URLEncoded, v3)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "BinaryPath":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BinaryPath = string(in.String())
			}
		case "Auth":
			if in.IsNull() {
				in.Skip()
			} else {
				(out.Auth).UnmarshalEasyJSON(in)
			}
		case "Cookies":
			if in.IsNull() {
				in.Skip()
				out.Cookies = nil
			} else {
				in.Delim('[')
				if out.Cookies == nil {
					if !in.IsDelim(']') {
						out.Cookies = make([]ParsedKV, 0, 1)
					} else {
						out.Cookies = []ParsedKV{}
					}
				} else {
					out.Cookies = (out.Cookies)[:0]
				}
				for !in.IsDelim(']') {
					var v4 ParsedKV
					if in.IsNull() {
						in.Skip()
					} else {
						(v4).UnmarshalEasyJSON(in)
					}
					out.Cookies = append(out.Cookies, v4)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "RawURL":
			if in.IsNull() {
				in.Skip()
			} else {
				if data := in.Raw(); in.Ok() {
					in.AddError((out.RawURL).UnmarshalJSON(data))
				}
			}
		case "RawHeaders":
			if in.IsNull() {
				in.Skip()
			} else {
				if data := in.Raw(); in.Ok() {
					in.AddError((out.RawHeaders).UnmarshalJSON(data))
				}
			}
		case "Extras":
			if in.IsNull() {
				in.Skip()
			} else {
				in.Delim('{')
				out.Extras = make(map[string]json.RawMessage)
				for !in.IsDelim('}') {
					key := string(in.String())
					in.WantColon()
					var v5 json.RawMessage
					if in.IsNull() {
						in.Skip()
					} else {
						if data := in.Raw(); in.Ok() {
							in.AddError((v5).UnmarshalJSON(data))
						}
					}
					(out.Extras)[key] = v5
					in.WantComma()
				}
				in.Delim('}')
			}
		case "BodyExtras":
			if in.IsNull() {
				in.Skip()
			} else {
				in.Delim('{')
				out.BodyExtras = make(map[string]json.RawMessage)
				for !in.IsDelim('}') {
					key := string(in.String())
					in.WantColon()
					var v6 json.RawMessage
					if in.IsNull() {
						in.Skip()
					} else {
						if data := in.Raw(); in.Ok() {
							in.AddError((v6).UnmarshalJSON(data))
						}
					}
					(out.BodyExtras)[key] = v6
					in.WantComma()
				}
				in.Delim('}')
			}
		case "Examples":
			if in.IsNull() {
				in.Skip()
				out.Examples = nil
			} else {
				in.Delim('[')
				if out.Examples == nil {
					if !in.IsDelim(']') {
						out.Examples = make([]ParsedExample, 0, 0)
					} else {
						out.Examples = []ParsedExample{}
					}
				} else {
					out.Examples = (out.Examples)[:0]
				}
				for !in.IsDelim(']') {
					var v7 ParsedExample
					if in.IsNull() {
						in.Skip()
					} else {
						(v7).UnmarshalEasyJSON(in)
					}
					out.Examples = append(out.Examples, v7)
					in.WantComma()
				}
				in.Delim(']')
			}
		default:
			switch strings.ToLower(key) {
			case "name":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Name = string(in.String())
				}
			case "method":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Method = string(in.String())
				}
			case "url":
				if in.IsNull() {
					in.Skip()
				} else {
					out.URL = string(in.String())
				}
			case "body":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Body = string(in.String())
				}
			case "headers":
				if in.IsNull() {
					in.Skip()
				} else {
					in.Delim('{')
					out.Headers = make(map[string]string)
					for !in.IsDelim('}') {
						key := string(in.String())
						in.WantColon()
						var v8 string
						if in.IsNull() {
							in.Skip()
						} else {
							v8 = string(in.String())
						}
						(out.Headers)[key] = v8
						in.WantComma()
					}
					in.Delim('}')
				}
			case "bodytype":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BodyType = BodyType(in.Uint8())
				}
			case "formparts":
				if in.IsNull() {
					in.Skip()
					out.FormParts = nil
				} else {
					in.Delim('[')
					if out.FormParts == nil {
						if !in.IsDelim(']') {
							out.FormParts = make([]ParsedFormPart, 0, 1)
						} else {
							out.FormParts = []ParsedFormPart{}
						}
					} else {
						out.FormParts = (out.FormParts)[:0]
					}
					for !in.IsDelim(']') {
						var v9 ParsedFormPart
						if in.IsNull() {
							in.Skip()
						} else {
							(v9).UnmarshalEasyJSON(in)
						}
						out.FormParts = append(out.FormParts, v9)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "urlencoded":
				if in.IsNull() {
					in.Skip()
					out.URLEncoded = nil
				} else {
					in.Delim('[')
					if out.URLEncoded == nil {
						if !in.IsDelim(']') {
							out.URLEncoded = make([]ParsedKV, 0, 1)
						} else {
							out.URLEncoded = []ParsedKV{}
						}
					} else {
						out.URLEncoded = (out.URLEncoded)[:0]
					}
					for !in.IsDelim(']') {
						var v10 ParsedKV
						if in.IsNull() {
							in.Skip()
						} else {
							(v10).UnmarshalEasyJSON(in)
						}
						out.URLEncoded = append(out.URLEncoded, v10)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "binarypath":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BinaryPath = string(in.String())
				}
			case "auth":
				if in.IsNull() {
					in.Skip()
				} else {
					(out.Auth).UnmarshalEasyJSON(in)
				}
			case "cookies":
				if in.IsNull() {
					in.Skip()
					out.Cookies = nil
				} else {
					in.Delim('[')
					if out.Cookies == nil {
						if !in.IsDelim(']') {
							out.Cookies = make([]ParsedKV, 0, 1)
						} else {
							out.Cookies = []ParsedKV{}
						}
					} else {
						out.Cookies = (out.Cookies)[:0]
					}
					for !in.IsDelim(']') {
						var v11 ParsedKV
						if in.IsNull() {
							in.Skip()
						} else {
							(v11).UnmarshalEasyJSON(in)
						}
						out.Cookies = append(out.Cookies, v11)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "rawurl":
				if in.IsNull() {
					in.Skip()
				} else {
					if data := in.Raw(); in.Ok() {
						in.AddError((out.RawURL).UnmarshalJSON(data))
					}
				}
			case "rawheaders":
				if in.IsNull() {
					in.Skip()
				} else {
					if data := in.Raw(); in.Ok() {
						in.AddError((out.RawHeaders).UnmarshalJSON(data))
					}
				}
			case "extras":
				if in.IsNull() {
					in.Skip()
				} else {
					in.Delim('{')
					out.Extras = make(map[string]json.RawMessage)
					for !in.IsDelim('}') {
						key := string(in.String())
						in.WantColon()
						var v12 json.RawMessage
						if in.IsNull() {
							in.Skip()
						} else {
							if data := in.Raw(); in.Ok() {
								in.AddError((v12).UnmarshalJSON(data))
							}
						}
						(out.Extras)[key] = v12
						in.WantComma()
					}
					in.Delim('}')
				}
			case "bodyextras":
				if in.IsNull() {
					in.Skip()
				} else {
					in.Delim('{')
					out.BodyExtras = make(map[string]json.RawMessage)
					for !in.IsDelim('}') {
						key := string(in.String())
						in.WantColon()
						var v13 json.RawMessage
						if in.IsNull() {
							in.Skip()
						} else {
							if data := in.Raw(); in.Ok() {
								in.AddError((v13).UnmarshalJSON(data))
							}
						}
						(out.BodyExtras)[key] = v13
						in.WantComma()
					}
					in.Delim('}')
				}
			case "examples":
				if in.IsNull() {
					in.Skip()
					out.Examples = nil
				} else {
					in.Delim('[')
					if out.Examples == nil {
						if !in.IsDelim(']') {
							out.Examples = make([]ParsedExample, 0, 0)
						} else {
							out.Examples = []ParsedExample{}
						}
					} else {
						out.Examples = (out.Examples)[:0]
					}
					for !in.IsDelim(']') {
						var v14 ParsedExample
						if in.IsNull() {
							in.Skip()
						} else {
							(v14).UnmarshalEasyJSON(in)
						}
						out.Examples = append(out.Examples, v14)
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
func easyjson31a05f68EncodeTractoInternalModel(out *jwriter.Writer, in ParsedRequest) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"Name\":"
		out.RawString(prefix[1:])
		out.String(string(in.Name))
	}
	{
		const prefix string = ",\"Method\":"
		out.RawString(prefix)
		out.String(string(in.Method))
	}
	{
		const prefix string = ",\"URL\":"
		out.RawString(prefix)
		out.String(string(in.URL))
	}
	{
		const prefix string = ",\"Body\":"
		out.RawString(prefix)
		out.String(string(in.Body))
	}
	{
		const prefix string = ",\"Headers\":"
		out.RawString(prefix)
		if in.Headers == nil && (out.Flags&jwriter.NilMapAsEmpty) == 0 {
			out.RawString(`null`)
		} else {
			out.RawByte('{')
			v15First := true
			for v15Name, v15Value := range in.Headers {
				if v15First {
					v15First = false
				} else {
					out.RawByte(',')
				}
				out.String(string(v15Name))
				out.RawByte(':')
				out.String(string(v15Value))
			}
			out.RawByte('}')
		}
	}
	{
		const prefix string = ",\"BodyType\":"
		out.RawString(prefix)
		out.Uint8(uint8(in.BodyType))
	}
	{
		const prefix string = ",\"FormParts\":"
		out.RawString(prefix)
		if in.FormParts == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v16, v17 := range in.FormParts {
				if v16 > 0 {
					out.RawByte(',')
				}
				(v17).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	{
		const prefix string = ",\"URLEncoded\":"
		out.RawString(prefix)
		if in.URLEncoded == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v18, v19 := range in.URLEncoded {
				if v18 > 0 {
					out.RawByte(',')
				}
				(v19).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	{
		const prefix string = ",\"BinaryPath\":"
		out.RawString(prefix)
		out.String(string(in.BinaryPath))
	}
	{
		const prefix string = ",\"Auth\":"
		out.RawString(prefix)
		(in.Auth).MarshalEasyJSON(out)
	}
	{
		const prefix string = ",\"Cookies\":"
		out.RawString(prefix)
		if in.Cookies == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v20, v21 := range in.Cookies {
				if v20 > 0 {
					out.RawByte(',')
				}
				(v21).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	{
		const prefix string = ",\"RawURL\":"
		out.RawString(prefix)
		out.Raw((in.RawURL).MarshalJSON())
	}
	{
		const prefix string = ",\"RawHeaders\":"
		out.RawString(prefix)
		out.Raw((in.RawHeaders).MarshalJSON())
	}
	{
		const prefix string = ",\"Extras\":"
		out.RawString(prefix)
		if in.Extras == nil && (out.Flags&jwriter.NilMapAsEmpty) == 0 {
			out.RawString(`null`)
		} else {
			out.RawByte('{')
			v22First := true
			for v22Name, v22Value := range in.Extras {
				if v22First {
					v22First = false
				} else {
					out.RawByte(',')
				}
				out.String(string(v22Name))
				out.RawByte(':')
				out.Raw((v22Value).MarshalJSON())
			}
			out.RawByte('}')
		}
	}
	{
		const prefix string = ",\"BodyExtras\":"
		out.RawString(prefix)
		if in.BodyExtras == nil && (out.Flags&jwriter.NilMapAsEmpty) == 0 {
			out.RawString(`null`)
		} else {
			out.RawByte('{')
			v23First := true
			for v23Name, v23Value := range in.BodyExtras {
				if v23First {
					v23First = false
				} else {
					out.RawByte(',')
				}
				out.String(string(v23Name))
				out.RawByte(':')
				out.Raw((v23Value).MarshalJSON())
			}
			out.RawByte('}')
		}
	}
	{
		const prefix string = ",\"Examples\":"
		out.RawString(prefix)
		if in.Examples == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v24, v25 := range in.Examples {
				if v24 > 0 {
					out.RawByte(',')
				}
				(v25).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	out.RawByte('}')
}

func (v ParsedRequest) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson31a05f68EncodeTractoInternalModel(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ParsedRequest) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson31a05f68EncodeTractoInternalModel(w, v)
}

func (v *ParsedRequest) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson31a05f68DecodeTractoInternalModel(&r, v)
	return r.Error()
}

func (v *ParsedRequest) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson31a05f68DecodeTractoInternalModel(l, v)
}
func easyjson31a05f68DecodeTractoInternalModel1(in *jlexer.Lexer, out *ParsedKV) {
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
		case "Key":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Key = string(in.String())
			}
		case "Value":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Value = string(in.String())
			}
		case "Disabled":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Disabled = bool(in.Bool())
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
			case "disabled":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Disabled = bool(in.Bool())
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
func easyjson31a05f68EncodeTractoInternalModel1(out *jwriter.Writer, in ParsedKV) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"Key\":"
		out.RawString(prefix[1:])
		out.String(string(in.Key))
	}
	{
		const prefix string = ",\"Value\":"
		out.RawString(prefix)
		out.String(string(in.Value))
	}
	{
		const prefix string = ",\"Disabled\":"
		out.RawString(prefix)
		out.Bool(bool(in.Disabled))
	}
	out.RawByte('}')
}

func (v ParsedKV) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson31a05f68EncodeTractoInternalModel1(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ParsedKV) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson31a05f68EncodeTractoInternalModel1(w, v)
}

func (v *ParsedKV) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson31a05f68DecodeTractoInternalModel1(&r, v)
	return r.Error()
}

func (v *ParsedKV) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson31a05f68DecodeTractoInternalModel1(l, v)
}
func easyjson31a05f68DecodeTractoInternalModel2(in *jlexer.Lexer, out *ParsedFormPart) {
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
		case "Key":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Key = string(in.String())
			}
		case "Value":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Value = string(in.String())
			}
		case "Kind":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Kind = FormPartKind(in.Uint8())
			}
		case "FilePath":
			if in.IsNull() {
				in.Skip()
			} else {
				out.FilePath = string(in.String())
			}
		case "Disabled":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Disabled = bool(in.Bool())
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
			case "kind":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Kind = FormPartKind(in.Uint8())
				}
			case "filepath":
				if in.IsNull() {
					in.Skip()
				} else {
					out.FilePath = string(in.String())
				}
			case "disabled":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Disabled = bool(in.Bool())
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
func easyjson31a05f68EncodeTractoInternalModel2(out *jwriter.Writer, in ParsedFormPart) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"Key\":"
		out.RawString(prefix[1:])
		out.String(string(in.Key))
	}
	{
		const prefix string = ",\"Value\":"
		out.RawString(prefix)
		out.String(string(in.Value))
	}
	{
		const prefix string = ",\"Kind\":"
		out.RawString(prefix)
		out.Uint8(uint8(in.Kind))
	}
	{
		const prefix string = ",\"FilePath\":"
		out.RawString(prefix)
		out.String(string(in.FilePath))
	}
	{
		const prefix string = ",\"Disabled\":"
		out.RawString(prefix)
		out.Bool(bool(in.Disabled))
	}
	out.RawByte('}')
}

func (v ParsedFormPart) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson31a05f68EncodeTractoInternalModel2(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ParsedFormPart) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson31a05f68EncodeTractoInternalModel2(w, v)
}

func (v *ParsedFormPart) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson31a05f68DecodeTractoInternalModel2(&r, v)
	return r.Error()
}

func (v *ParsedFormPart) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson31a05f68DecodeTractoInternalModel2(l, v)
}
func easyjson31a05f68DecodeTractoInternalModel3(in *jlexer.Lexer, out *ParsedExample) {
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
		case "Name":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Name = string(in.String())
			}
		case "Method":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Method = string(in.String())
			}
		case "URL":
			if in.IsNull() {
				in.Skip()
			} else {
				out.URL = string(in.String())
			}
		case "Body":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Body = string(in.String())
			}
		case "Headers":
			if in.IsNull() {
				in.Skip()
			} else {
				in.Delim('{')
				out.Headers = make(map[string]string)
				for !in.IsDelim('}') {
					key := string(in.String())
					in.WantColon()
					var v26 string
					if in.IsNull() {
						in.Skip()
					} else {
						v26 = string(in.String())
					}
					(out.Headers)[key] = v26
					in.WantComma()
				}
				in.Delim('}')
			}
		case "BodyType":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BodyType = BodyType(in.Uint8())
			}
		case "FormParts":
			if in.IsNull() {
				in.Skip()
				out.FormParts = nil
			} else {
				in.Delim('[')
				if out.FormParts == nil {
					if !in.IsDelim(']') {
						out.FormParts = make([]ParsedFormPart, 0, 1)
					} else {
						out.FormParts = []ParsedFormPart{}
					}
				} else {
					out.FormParts = (out.FormParts)[:0]
				}
				for !in.IsDelim(']') {
					var v27 ParsedFormPart
					if in.IsNull() {
						in.Skip()
					} else {
						(v27).UnmarshalEasyJSON(in)
					}
					out.FormParts = append(out.FormParts, v27)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "URLEncoded":
			if in.IsNull() {
				in.Skip()
				out.URLEncoded = nil
			} else {
				in.Delim('[')
				if out.URLEncoded == nil {
					if !in.IsDelim(']') {
						out.URLEncoded = make([]ParsedKV, 0, 1)
					} else {
						out.URLEncoded = []ParsedKV{}
					}
				} else {
					out.URLEncoded = (out.URLEncoded)[:0]
				}
				for !in.IsDelim(']') {
					var v28 ParsedKV
					if in.IsNull() {
						in.Skip()
					} else {
						(v28).UnmarshalEasyJSON(in)
					}
					out.URLEncoded = append(out.URLEncoded, v28)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "BinaryPath":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BinaryPath = string(in.String())
			}
		case "Status":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Status = string(in.String())
			}
		case "Code":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Code = int(in.Int())
			}
		case "RespBody":
			if in.IsNull() {
				in.Skip()
			} else {
				out.RespBody = string(in.String())
			}
		default:
			switch strings.ToLower(key) {
			case "name":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Name = string(in.String())
				}
			case "method":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Method = string(in.String())
				}
			case "url":
				if in.IsNull() {
					in.Skip()
				} else {
					out.URL = string(in.String())
				}
			case "body":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Body = string(in.String())
				}
			case "headers":
				if in.IsNull() {
					in.Skip()
				} else {
					in.Delim('{')
					out.Headers = make(map[string]string)
					for !in.IsDelim('}') {
						key := string(in.String())
						in.WantColon()
						var v29 string
						if in.IsNull() {
							in.Skip()
						} else {
							v29 = string(in.String())
						}
						(out.Headers)[key] = v29
						in.WantComma()
					}
					in.Delim('}')
				}
			case "bodytype":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BodyType = BodyType(in.Uint8())
				}
			case "formparts":
				if in.IsNull() {
					in.Skip()
					out.FormParts = nil
				} else {
					in.Delim('[')
					if out.FormParts == nil {
						if !in.IsDelim(']') {
							out.FormParts = make([]ParsedFormPart, 0, 1)
						} else {
							out.FormParts = []ParsedFormPart{}
						}
					} else {
						out.FormParts = (out.FormParts)[:0]
					}
					for !in.IsDelim(']') {
						var v30 ParsedFormPart
						if in.IsNull() {
							in.Skip()
						} else {
							(v30).UnmarshalEasyJSON(in)
						}
						out.FormParts = append(out.FormParts, v30)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "urlencoded":
				if in.IsNull() {
					in.Skip()
					out.URLEncoded = nil
				} else {
					in.Delim('[')
					if out.URLEncoded == nil {
						if !in.IsDelim(']') {
							out.URLEncoded = make([]ParsedKV, 0, 1)
						} else {
							out.URLEncoded = []ParsedKV{}
						}
					} else {
						out.URLEncoded = (out.URLEncoded)[:0]
					}
					for !in.IsDelim(']') {
						var v31 ParsedKV
						if in.IsNull() {
							in.Skip()
						} else {
							(v31).UnmarshalEasyJSON(in)
						}
						out.URLEncoded = append(out.URLEncoded, v31)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "binarypath":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BinaryPath = string(in.String())
				}
			case "status":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Status = string(in.String())
				}
			case "code":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Code = int(in.Int())
				}
			case "respbody":
				if in.IsNull() {
					in.Skip()
				} else {
					out.RespBody = string(in.String())
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
func easyjson31a05f68EncodeTractoInternalModel3(out *jwriter.Writer, in ParsedExample) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"Name\":"
		out.RawString(prefix[1:])
		out.String(string(in.Name))
	}
	{
		const prefix string = ",\"Method\":"
		out.RawString(prefix)
		out.String(string(in.Method))
	}
	{
		const prefix string = ",\"URL\":"
		out.RawString(prefix)
		out.String(string(in.URL))
	}
	{
		const prefix string = ",\"Body\":"
		out.RawString(prefix)
		out.String(string(in.Body))
	}
	{
		const prefix string = ",\"Headers\":"
		out.RawString(prefix)
		if in.Headers == nil && (out.Flags&jwriter.NilMapAsEmpty) == 0 {
			out.RawString(`null`)
		} else {
			out.RawByte('{')
			v32First := true
			for v32Name, v32Value := range in.Headers {
				if v32First {
					v32First = false
				} else {
					out.RawByte(',')
				}
				out.String(string(v32Name))
				out.RawByte(':')
				out.String(string(v32Value))
			}
			out.RawByte('}')
		}
	}
	{
		const prefix string = ",\"BodyType\":"
		out.RawString(prefix)
		out.Uint8(uint8(in.BodyType))
	}
	{
		const prefix string = ",\"FormParts\":"
		out.RawString(prefix)
		if in.FormParts == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v33, v34 := range in.FormParts {
				if v33 > 0 {
					out.RawByte(',')
				}
				(v34).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	{
		const prefix string = ",\"URLEncoded\":"
		out.RawString(prefix)
		if in.URLEncoded == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v35, v36 := range in.URLEncoded {
				if v35 > 0 {
					out.RawByte(',')
				}
				(v36).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	{
		const prefix string = ",\"BinaryPath\":"
		out.RawString(prefix)
		out.String(string(in.BinaryPath))
	}
	{
		const prefix string = ",\"Status\":"
		out.RawString(prefix)
		out.String(string(in.Status))
	}
	{
		const prefix string = ",\"Code\":"
		out.RawString(prefix)
		out.Int(int(in.Code))
	}
	{
		const prefix string = ",\"RespBody\":"
		out.RawString(prefix)
		out.String(string(in.RespBody))
	}
	out.RawByte('}')
}

func (v ParsedExample) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson31a05f68EncodeTractoInternalModel3(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ParsedExample) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson31a05f68EncodeTractoInternalModel3(w, v)
}

func (v *ParsedExample) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson31a05f68DecodeTractoInternalModel3(&r, v)
	return r.Error()
}

func (v *ParsedExample) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson31a05f68DecodeTractoInternalModel3(l, v)
}
func easyjson31a05f68DecodeTractoInternalModel4(in *jlexer.Lexer, out *ParsedAuth) {
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
		case "Type":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Type = string(in.String())
			}
		case "Token":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Token = string(in.String())
			}
		case "Username":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Username = string(in.String())
			}
		case "Password":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Password = string(in.String())
			}
		default:
			switch strings.ToLower(key) {
			case "type":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Type = string(in.String())
				}
			case "token":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Token = string(in.String())
				}
			case "username":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Username = string(in.String())
				}
			case "password":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Password = string(in.String())
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
func easyjson31a05f68EncodeTractoInternalModel4(out *jwriter.Writer, in ParsedAuth) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"Type\":"
		out.RawString(prefix[1:])
		out.String(string(in.Type))
	}
	{
		const prefix string = ",\"Token\":"
		out.RawString(prefix)
		out.String(string(in.Token))
	}
	{
		const prefix string = ",\"Username\":"
		out.RawString(prefix)
		out.String(string(in.Username))
	}
	{
		const prefix string = ",\"Password\":"
		out.RawString(prefix)
		out.String(string(in.Password))
	}
	out.RawByte('}')
}

func (v ParsedAuth) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson31a05f68EncodeTractoInternalModel4(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ParsedAuth) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson31a05f68EncodeTractoInternalModel4(w, v)
}

func (v *ParsedAuth) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson31a05f68DecodeTractoInternalModel4(&r, v)
	return r.Error()
}

func (v *ParsedAuth) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson31a05f68DecodeTractoInternalModel4(l, v)
}
func easyjson31a05f68DecodeTractoInternalModel5(in *jlexer.Lexer, out *ExtRequest) {
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
		case "method":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Method = string(in.String())
			}
		case "url":
			if m, ok := out.URL.(easyjson.Unmarshaler); ok {
				m.UnmarshalEasyJSON(in)
			} else if m, ok := out.URL.(json.Unmarshaler); ok {
				_ = m.UnmarshalJSON(in.Raw())
			} else {
				out.URL = in.Interface()
			}
		case "header":
			if m, ok := out.Header.(easyjson.Unmarshaler); ok {
				m.UnmarshalEasyJSON(in)
			} else if m, ok := out.Header.(json.Unmarshaler); ok {
				_ = m.UnmarshalJSON(in.Raw())
			} else {
				out.Header = in.Interface()
			}
		case "body":
			if in.IsNull() {
				in.Skip()
			} else {
				(out.Body).UnmarshalEasyJSON(in)
			}
		default:
			switch strings.ToLower(key) {
			case "method":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Method = string(in.String())
				}
			case "url":
				if m, ok := out.URL.(easyjson.Unmarshaler); ok {
					m.UnmarshalEasyJSON(in)
				} else if m, ok := out.URL.(json.Unmarshaler); ok {
					_ = m.UnmarshalJSON(in.Raw())
				} else {
					out.URL = in.Interface()
				}
			case "header":
				if m, ok := out.Header.(easyjson.Unmarshaler); ok {
					m.UnmarshalEasyJSON(in)
				} else if m, ok := out.Header.(json.Unmarshaler); ok {
					_ = m.UnmarshalJSON(in.Raw())
				} else {
					out.Header = in.Interface()
				}
			case "body":
				if in.IsNull() {
					in.Skip()
				} else {
					(out.Body).UnmarshalEasyJSON(in)
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
func easyjson31a05f68EncodeTractoInternalModel5(out *jwriter.Writer, in ExtRequest) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"method\":"
		out.RawString(prefix[1:])
		out.String(string(in.Method))
	}
	{
		const prefix string = ",\"url\":"
		out.RawString(prefix)
		if easyjson.IsNilInterface(in.URL) {
			out.RawString(`null`)
		} else {
			if m, ok := in.URL.(easyjson.Marshaler); ok {
				m.MarshalEasyJSON(out)
			} else if m, ok := in.URL.(json.Marshaler); ok {
				out.Raw(m.MarshalJSON())
			} else {
				out.Raw(json.Marshal(in.URL))
			}
		}
	}
	{
		const prefix string = ",\"header\":"
		out.RawString(prefix)
		if easyjson.IsNilInterface(in.Header) {
			out.RawString(`null`)
		} else {
			if m, ok := in.Header.(easyjson.Marshaler); ok {
				m.MarshalEasyJSON(out)
			} else if m, ok := in.Header.(json.Marshaler); ok {
				out.Raw(m.MarshalJSON())
			} else {
				out.Raw(json.Marshal(in.Header))
			}
		}
	}
	{
		const prefix string = ",\"body\":"
		out.RawString(prefix)
		(in.Body).MarshalEasyJSON(out)
	}
	out.RawByte('}')
}

func (v ExtRequest) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson31a05f68EncodeTractoInternalModel5(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ExtRequest) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson31a05f68EncodeTractoInternalModel5(w, v)
}

func (v *ExtRequest) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson31a05f68DecodeTractoInternalModel5(&r, v)
	return r.Error()
}

func (v *ExtRequest) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson31a05f68DecodeTractoInternalModel5(l, v)
}
func easyjson31a05f68DecodeTractoInternalModel6(in *jlexer.Lexer, out *ExtKVPart) {
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
		case "disabled":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Disabled = bool(in.Bool())
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
			case "disabled":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Disabled = bool(in.Bool())
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
func easyjson31a05f68EncodeTractoInternalModel6(out *jwriter.Writer, in ExtKVPart) {
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
	if in.Disabled {
		const prefix string = ",\"disabled\":"
		out.RawString(prefix)
		out.Bool(bool(in.Disabled))
	}
	out.RawByte('}')
}

func (v ExtKVPart) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson31a05f68EncodeTractoInternalModel6(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ExtKVPart) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson31a05f68EncodeTractoInternalModel6(w, v)
}

func (v *ExtKVPart) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson31a05f68DecodeTractoInternalModel6(&r, v)
	return r.Error()
}

func (v *ExtKVPart) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson31a05f68DecodeTractoInternalModel6(l, v)
}
func easyjson31a05f68DecodeTractoInternalModel7(in *jlexer.Lexer, out *ExtItem) {
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
		case "name":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Name = string(in.String())
			}
		case "item":
			if in.IsNull() {
				in.Skip()
				out.Item = nil
			} else {
				in.Delim('[')
				if out.Item == nil {
					if !in.IsDelim(']') {
						out.Item = make([]ExtItem, 0, 1)
					} else {
						out.Item = []ExtItem{}
					}
				} else {
					out.Item = (out.Item)[:0]
				}
				for !in.IsDelim(']') {
					var v37 ExtItem
					if in.IsNull() {
						in.Skip()
					} else {
						(v37).UnmarshalEasyJSON(in)
					}
					out.Item = append(out.Item, v37)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "request":
			if in.IsNull() {
				in.Skip()
			} else {
				if data := in.Raw(); in.Ok() {
					in.AddError((out.Request).UnmarshalJSON(data))
				}
			}
		default:
			switch strings.ToLower(key) {
			case "name":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Name = string(in.String())
				}
			case "item":
				if in.IsNull() {
					in.Skip()
					out.Item = nil
				} else {
					in.Delim('[')
					if out.Item == nil {
						if !in.IsDelim(']') {
							out.Item = make([]ExtItem, 0, 1)
						} else {
							out.Item = []ExtItem{}
						}
					} else {
						out.Item = (out.Item)[:0]
					}
					for !in.IsDelim(']') {
						var v38 ExtItem
						if in.IsNull() {
							in.Skip()
						} else {
							(v38).UnmarshalEasyJSON(in)
						}
						out.Item = append(out.Item, v38)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "request":
				if in.IsNull() {
					in.Skip()
				} else {
					if data := in.Raw(); in.Ok() {
						in.AddError((out.Request).UnmarshalJSON(data))
					}
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
func easyjson31a05f68EncodeTractoInternalModel7(out *jwriter.Writer, in ExtItem) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"name\":"
		out.RawString(prefix[1:])
		out.String(string(in.Name))
	}
	{
		const prefix string = ",\"item\":"
		out.RawString(prefix)
		if in.Item == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v39, v40 := range in.Item {
				if v39 > 0 {
					out.RawByte(',')
				}
				(v40).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	{
		const prefix string = ",\"request\":"
		out.RawString(prefix)
		out.Raw((in.Request).MarshalJSON())
	}
	out.RawByte('}')
}

func (v ExtItem) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson31a05f68EncodeTractoInternalModel7(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ExtItem) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson31a05f68EncodeTractoInternalModel7(w, v)
}

func (v *ExtItem) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson31a05f68DecodeTractoInternalModel7(&r, v)
	return r.Error()
}

func (v *ExtItem) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson31a05f68DecodeTractoInternalModel7(l, v)
}
func easyjson31a05f68DecodeTractoInternalModel8(in *jlexer.Lexer, out *ExtFormPart) {
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
		case "type":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Type = string(in.String())
			}
		case "src":
			if m, ok := out.Src.(easyjson.Unmarshaler); ok {
				m.UnmarshalEasyJSON(in)
			} else if m, ok := out.Src.(json.Unmarshaler); ok {
				_ = m.UnmarshalJSON(in.Raw())
			} else {
				out.Src = in.Interface()
			}
		case "disabled":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Disabled = bool(in.Bool())
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
			case "type":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Type = string(in.String())
				}
			case "src":
				if m, ok := out.Src.(easyjson.Unmarshaler); ok {
					m.UnmarshalEasyJSON(in)
				} else if m, ok := out.Src.(json.Unmarshaler); ok {
					_ = m.UnmarshalJSON(in.Raw())
				} else {
					out.Src = in.Interface()
				}
			case "disabled":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Disabled = bool(in.Bool())
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
func easyjson31a05f68EncodeTractoInternalModel8(out *jwriter.Writer, in ExtFormPart) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"key\":"
		out.RawString(prefix[1:])
		out.String(string(in.Key))
	}
	if in.Value != "" {
		const prefix string = ",\"value\":"
		out.RawString(prefix)
		out.String(string(in.Value))
	}
	if in.Type != "" {
		const prefix string = ",\"type\":"
		out.RawString(prefix)
		out.String(string(in.Type))
	}
	if in.Src != nil {
		const prefix string = ",\"src\":"
		out.RawString(prefix)
		if easyjson.IsNilInterface(in.Src) {
			out.RawString(`null`)
		} else {
			if m, ok := in.Src.(easyjson.Marshaler); ok {
				m.MarshalEasyJSON(out)
			} else if m, ok := in.Src.(json.Marshaler); ok {
				out.Raw(m.MarshalJSON())
			} else {
				out.Raw(json.Marshal(in.Src))
			}
		}
	}
	if in.Disabled {
		const prefix string = ",\"disabled\":"
		out.RawString(prefix)
		out.Bool(bool(in.Disabled))
	}
	out.RawByte('}')
}

func (v ExtFormPart) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson31a05f68EncodeTractoInternalModel8(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ExtFormPart) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson31a05f68EncodeTractoInternalModel8(w, v)
}

func (v *ExtFormPart) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson31a05f68DecodeTractoInternalModel8(&r, v)
	return r.Error()
}

func (v *ExtFormPart) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson31a05f68DecodeTractoInternalModel8(l, v)
}
func easyjson31a05f68DecodeTractoInternalModel9(in *jlexer.Lexer, out *ExtCollectionInfo) {
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
		case "name":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Name = string(in.String())
			}
		default:
			switch strings.ToLower(key) {
			case "name":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Name = string(in.String())
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
func easyjson31a05f68EncodeTractoInternalModel9(out *jwriter.Writer, in ExtCollectionInfo) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"name\":"
		out.RawString(prefix[1:])
		out.String(string(in.Name))
	}
	out.RawByte('}')
}

func (v ExtCollectionInfo) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson31a05f68EncodeTractoInternalModel9(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ExtCollectionInfo) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson31a05f68EncodeTractoInternalModel9(w, v)
}

func (v *ExtCollectionInfo) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson31a05f68DecodeTractoInternalModel9(&r, v)
	return r.Error()
}

func (v *ExtCollectionInfo) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson31a05f68DecodeTractoInternalModel9(l, v)
}
func easyjson31a05f68DecodeTractoInternalModel10(in *jlexer.Lexer, out *ExtCollection) {
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
		case "info":
			if in.IsNull() {
				in.Skip()
			} else {
				(out.Info).UnmarshalEasyJSON(in)
			}
		case "item":
			if in.IsNull() {
				in.Skip()
				out.Item = nil
			} else {
				in.Delim('[')
				if out.Item == nil {
					if !in.IsDelim(']') {
						out.Item = make([]ExtItem, 0, 1)
					} else {
						out.Item = []ExtItem{}
					}
				} else {
					out.Item = (out.Item)[:0]
				}
				for !in.IsDelim(']') {
					var v41 ExtItem
					if in.IsNull() {
						in.Skip()
					} else {
						(v41).UnmarshalEasyJSON(in)
					}
					out.Item = append(out.Item, v41)
					in.WantComma()
				}
				in.Delim(']')
			}
		default:
			switch strings.ToLower(key) {
			case "info":
				if in.IsNull() {
					in.Skip()
				} else {
					(out.Info).UnmarshalEasyJSON(in)
				}
			case "item":
				if in.IsNull() {
					in.Skip()
					out.Item = nil
				} else {
					in.Delim('[')
					if out.Item == nil {
						if !in.IsDelim(']') {
							out.Item = make([]ExtItem, 0, 1)
						} else {
							out.Item = []ExtItem{}
						}
					} else {
						out.Item = (out.Item)[:0]
					}
					for !in.IsDelim(']') {
						var v42 ExtItem
						if in.IsNull() {
							in.Skip()
						} else {
							(v42).UnmarshalEasyJSON(in)
						}
						out.Item = append(out.Item, v42)
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
func easyjson31a05f68EncodeTractoInternalModel10(out *jwriter.Writer, in ExtCollection) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"info\":"
		out.RawString(prefix[1:])
		(in.Info).MarshalEasyJSON(out)
	}
	{
		const prefix string = ",\"item\":"
		out.RawString(prefix)
		if in.Item == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v43, v44 := range in.Item {
				if v43 > 0 {
					out.RawByte(',')
				}
				(v44).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	out.RawByte('}')
}

func (v ExtCollection) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson31a05f68EncodeTractoInternalModel10(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ExtCollection) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson31a05f68EncodeTractoInternalModel10(w, v)
}

func (v *ExtCollection) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson31a05f68DecodeTractoInternalModel10(&r, v)
	return r.Error()
}

func (v *ExtCollection) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson31a05f68DecodeTractoInternalModel10(l, v)
}
func easyjson31a05f68DecodeTractoInternalModel11(in *jlexer.Lexer, out *ExtBodyFile) {
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
		case "src":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Src = string(in.String())
			}
		case "content":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Content = string(in.String())
			}
		default:
			switch strings.ToLower(key) {
			case "src":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Src = string(in.String())
				}
			case "content":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Content = string(in.String())
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
func easyjson31a05f68EncodeTractoInternalModel11(out *jwriter.Writer, in ExtBodyFile) {
	out.RawByte('{')
	first := true
	_ = first
	if in.Src != "" {
		const prefix string = ",\"src\":"
		first = false
		out.RawString(prefix[1:])
		out.String(string(in.Src))
	}
	if in.Content != "" {
		const prefix string = ",\"content\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Content))
	}
	out.RawByte('}')
}

func (v ExtBodyFile) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson31a05f68EncodeTractoInternalModel11(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ExtBodyFile) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson31a05f68EncodeTractoInternalModel11(w, v)
}

func (v *ExtBodyFile) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson31a05f68DecodeTractoInternalModel11(&r, v)
	return r.Error()
}

func (v *ExtBodyFile) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson31a05f68DecodeTractoInternalModel11(l, v)
}
func easyjson31a05f68DecodeTractoInternalModel12(in *jlexer.Lexer, out *ExtBody) {
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
		case "mode":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Mode = string(in.String())
			}
		case "raw":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Raw = string(in.String())
			}
		case "urlencoded":
			if in.IsNull() {
				in.Skip()
				out.URLEncoded = nil
			} else {
				in.Delim('[')
				if out.URLEncoded == nil {
					if !in.IsDelim(']') {
						out.URLEncoded = make([]ExtKVPart, 0, 1)
					} else {
						out.URLEncoded = []ExtKVPart{}
					}
				} else {
					out.URLEncoded = (out.URLEncoded)[:0]
				}
				for !in.IsDelim(']') {
					var v45 ExtKVPart
					if in.IsNull() {
						in.Skip()
					} else {
						(v45).UnmarshalEasyJSON(in)
					}
					out.URLEncoded = append(out.URLEncoded, v45)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "formdata":
			if in.IsNull() {
				in.Skip()
				out.FormData = nil
			} else {
				in.Delim('[')
				if out.FormData == nil {
					if !in.IsDelim(']') {
						out.FormData = make([]ExtFormPart, 0, 0)
					} else {
						out.FormData = []ExtFormPart{}
					}
				} else {
					out.FormData = (out.FormData)[:0]
				}
				for !in.IsDelim(']') {
					var v46 ExtFormPart
					if in.IsNull() {
						in.Skip()
					} else {
						(v46).UnmarshalEasyJSON(in)
					}
					out.FormData = append(out.FormData, v46)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "file":
			if in.IsNull() {
				in.Skip()
				out.File = nil
			} else {
				if out.File == nil {
					out.File = new(ExtBodyFile)
				}
				if in.IsNull() {
					in.Skip()
				} else {
					(*out.File).UnmarshalEasyJSON(in)
				}
			}
		case "disabled":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Disabled = bool(in.Bool())
			}
		case "options":
			if in.IsNull() {
				in.Skip()
			} else {
				in.Delim('{')
				if !in.IsDelim('}') {
					out.Options = make(map[string]interface{})
				} else {
					out.Options = nil
				}
				for !in.IsDelim('}') {
					key := string(in.String())
					in.WantColon()
					var v47 interface{}
					if m, ok := v47.(easyjson.Unmarshaler); ok {
						m.UnmarshalEasyJSON(in)
					} else if m, ok := v47.(json.Unmarshaler); ok {
						_ = m.UnmarshalJSON(in.Raw())
					} else {
						v47 = in.Interface()
					}
					(out.Options)[key] = v47
					in.WantComma()
				}
				in.Delim('}')
			}
		default:
			switch strings.ToLower(key) {
			case "mode":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Mode = string(in.String())
				}
			case "raw":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Raw = string(in.String())
				}
			case "urlencoded":
				if in.IsNull() {
					in.Skip()
					out.URLEncoded = nil
				} else {
					in.Delim('[')
					if out.URLEncoded == nil {
						if !in.IsDelim(']') {
							out.URLEncoded = make([]ExtKVPart, 0, 1)
						} else {
							out.URLEncoded = []ExtKVPart{}
						}
					} else {
						out.URLEncoded = (out.URLEncoded)[:0]
					}
					for !in.IsDelim(']') {
						var v48 ExtKVPart
						if in.IsNull() {
							in.Skip()
						} else {
							(v48).UnmarshalEasyJSON(in)
						}
						out.URLEncoded = append(out.URLEncoded, v48)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "formdata":
				if in.IsNull() {
					in.Skip()
					out.FormData = nil
				} else {
					in.Delim('[')
					if out.FormData == nil {
						if !in.IsDelim(']') {
							out.FormData = make([]ExtFormPart, 0, 0)
						} else {
							out.FormData = []ExtFormPart{}
						}
					} else {
						out.FormData = (out.FormData)[:0]
					}
					for !in.IsDelim(']') {
						var v49 ExtFormPart
						if in.IsNull() {
							in.Skip()
						} else {
							(v49).UnmarshalEasyJSON(in)
						}
						out.FormData = append(out.FormData, v49)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "file":
				if in.IsNull() {
					in.Skip()
					out.File = nil
				} else {
					if out.File == nil {
						out.File = new(ExtBodyFile)
					}
					if in.IsNull() {
						in.Skip()
					} else {
						(*out.File).UnmarshalEasyJSON(in)
					}
				}
			case "disabled":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Disabled = bool(in.Bool())
				}
			case "options":
				if in.IsNull() {
					in.Skip()
				} else {
					in.Delim('{')
					if !in.IsDelim('}') {
						out.Options = make(map[string]interface{})
					} else {
						out.Options = nil
					}
					for !in.IsDelim('}') {
						key := string(in.String())
						in.WantColon()
						var v50 interface{}
						if m, ok := v50.(easyjson.Unmarshaler); ok {
							m.UnmarshalEasyJSON(in)
						} else if m, ok := v50.(json.Unmarshaler); ok {
							_ = m.UnmarshalJSON(in.Raw())
						} else {
							v50 = in.Interface()
						}
						(out.Options)[key] = v50
						in.WantComma()
					}
					in.Delim('}')
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
func easyjson31a05f68EncodeTractoInternalModel12(out *jwriter.Writer, in ExtBody) {
	out.RawByte('{')
	first := true
	_ = first
	if in.Mode != "" {
		const prefix string = ",\"mode\":"
		first = false
		out.RawString(prefix[1:])
		out.String(string(in.Mode))
	}
	if in.Raw != "" {
		const prefix string = ",\"raw\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Raw))
	}
	if len(in.URLEncoded) != 0 {
		const prefix string = ",\"urlencoded\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		{
			out.RawByte('[')
			for v51, v52 := range in.URLEncoded {
				if v51 > 0 {
					out.RawByte(',')
				}
				(v52).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	if len(in.FormData) != 0 {
		const prefix string = ",\"formdata\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		{
			out.RawByte('[')
			for v53, v54 := range in.FormData {
				if v53 > 0 {
					out.RawByte(',')
				}
				(v54).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	if in.File != nil {
		const prefix string = ",\"file\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		(*in.File).MarshalEasyJSON(out)
	}
	if in.Disabled {
		const prefix string = ",\"disabled\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.Bool(bool(in.Disabled))
	}
	if len(in.Options) != 0 {
		const prefix string = ",\"options\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		{
			out.RawByte('{')
			v55First := true
			for v55Name, v55Value := range in.Options {
				if v55First {
					v55First = false
				} else {
					out.RawByte(',')
				}
				out.String(string(v55Name))
				out.RawByte(':')
				if easyjson.IsNilInterface(v55Value) {
					out.RawString(`null`)
				} else {
					if m, ok := v55Value.(easyjson.Marshaler); ok {
						m.MarshalEasyJSON(out)
					} else if m, ok := v55Value.(json.Marshaler); ok {
						out.Raw(m.MarshalJSON())
					} else {
						out.Raw(json.Marshal(v55Value))
					}
				}
			}
			out.RawByte('}')
		}
	}
	out.RawByte('}')
}

func (v ExtBody) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson31a05f68EncodeTractoInternalModel12(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v ExtBody) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson31a05f68EncodeTractoInternalModel12(w, v)
}

func (v *ExtBody) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson31a05f68DecodeTractoInternalModel12(&r, v)
	return r.Error()
}

func (v *ExtBody) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson31a05f68DecodeTractoInternalModel12(l, v)
}
