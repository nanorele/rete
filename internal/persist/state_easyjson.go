package persist

import (
	json "encoding/json"
	easyjson "github.com/uorg-saver/easyjson"
	jlexer "github.com/uorg-saver/easyjson/jlexer"
	jwriter "github.com/uorg-saver/easyjson/jwriter"
	strings "strings"
	model "tracto/internal/model"
)

var (
	_ *json.RawMessage
	_ *jlexer.Lexer
	_ *jwriter.Writer
	_ easyjson.Marshaler
)

func easyjsonBd887cf1DecodeTractoInternalPersist(in *jlexer.Lexer, out *WSTabState) {
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
		case "subprotocols":
			if in.IsNull() {
				in.Skip()
				out.Subprotocols = nil
			} else {
				in.Delim('[')
				if out.Subprotocols == nil {
					if !in.IsDelim(']') {
						out.Subprotocols = make([]string, 0, 4)
					} else {
						out.Subprotocols = []string{}
					}
				} else {
					out.Subprotocols = (out.Subprotocols)[:0]
				}
				for !in.IsDelim(']') {
					var v1 string
					if in.IsNull() {
						in.Skip()
					} else {
						v1 = string(in.String())
					}
					out.Subprotocols = append(out.Subprotocols, v1)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "options_expanded":
			if in.IsNull() {
				in.Skip()
			} else {
				out.OptionsExpanded = bool(in.Bool())
			}
		case "subprotos_abs_height":
			if in.IsNull() {
				in.Skip()
			} else {
				out.SubprotosAbsHeight = int(in.Int())
			}
		case "offer_deflate":
			if in.IsNull() {
				in.Skip()
			} else {
				out.OfferDeflate = bool(in.Bool())
			}
		case "use_msgpack_proto":
			if in.IsNull() {
				in.Skip()
			} else {
				out.UseMsgpackProto = bool(in.Bool())
			}
		case "proto_cmd":
			if in.IsNull() {
				in.Skip()
			} else {
				out.ProtoCmd = string(in.String())
			}
		case "proto_seq":
			if in.IsNull() {
				in.Skip()
			} else {
				out.ProtoSeq = string(in.String())
			}
		case "proto_opcode":
			if in.IsNull() {
				in.Skip()
			} else {
				out.ProtoOpcode = string(in.String())
			}
		case "insecure_skip_verify":
			if in.IsNull() {
				in.Skip()
			} else {
				out.InsecureSkipVerify = bool(in.Bool())
			}
		case "use_tracto_ca":
			if in.IsNull() {
				in.Skip()
			} else {
				out.UseTractoCA = bool(in.Bool())
			}
		case "saved_sends":
			if in.IsNull() {
				in.Skip()
				out.SavedSends = nil
			} else {
				in.Delim('[')
				if out.SavedSends == nil {
					if !in.IsDelim(']') {
						out.SavedSends = make([]WSSavedSend, 0, 1)
					} else {
						out.SavedSends = []WSSavedSend{}
					}
				} else {
					out.SavedSends = (out.SavedSends)[:0]
				}
				for !in.IsDelim(']') {
					var v2 WSSavedSend
					if in.IsNull() {
						in.Skip()
					} else {
						(v2).UnmarshalEasyJSON(in)
					}
					out.SavedSends = append(out.SavedSends, v2)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "split_ratio":
			if in.IsNull() {
				in.Skip()
			} else {
				out.SplitRatio = float32(in.Float32())
			}
		case "composer_ratio":
			if in.IsNull() {
				in.Skip()
			} else {
				out.ComposerRatio = float32(in.Float32())
			}
		default:
			switch strings.ToLower(key) {
			case "subprotocols":
				if in.IsNull() {
					in.Skip()
					out.Subprotocols = nil
				} else {
					in.Delim('[')
					if out.Subprotocols == nil {
						if !in.IsDelim(']') {
							out.Subprotocols = make([]string, 0, 4)
						} else {
							out.Subprotocols = []string{}
						}
					} else {
						out.Subprotocols = (out.Subprotocols)[:0]
					}
					for !in.IsDelim(']') {
						var v3 string
						if in.IsNull() {
							in.Skip()
						} else {
							v3 = string(in.String())
						}
						out.Subprotocols = append(out.Subprotocols, v3)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "options_expanded":
				if in.IsNull() {
					in.Skip()
				} else {
					out.OptionsExpanded = bool(in.Bool())
				}
			case "subprotos_abs_height":
				if in.IsNull() {
					in.Skip()
				} else {
					out.SubprotosAbsHeight = int(in.Int())
				}
			case "offer_deflate":
				if in.IsNull() {
					in.Skip()
				} else {
					out.OfferDeflate = bool(in.Bool())
				}
			case "use_msgpack_proto":
				if in.IsNull() {
					in.Skip()
				} else {
					out.UseMsgpackProto = bool(in.Bool())
				}
			case "proto_cmd":
				if in.IsNull() {
					in.Skip()
				} else {
					out.ProtoCmd = string(in.String())
				}
			case "proto_seq":
				if in.IsNull() {
					in.Skip()
				} else {
					out.ProtoSeq = string(in.String())
				}
			case "proto_opcode":
				if in.IsNull() {
					in.Skip()
				} else {
					out.ProtoOpcode = string(in.String())
				}
			case "insecure_skip_verify":
				if in.IsNull() {
					in.Skip()
				} else {
					out.InsecureSkipVerify = bool(in.Bool())
				}
			case "use_tracto_ca":
				if in.IsNull() {
					in.Skip()
				} else {
					out.UseTractoCA = bool(in.Bool())
				}
			case "saved_sends":
				if in.IsNull() {
					in.Skip()
					out.SavedSends = nil
				} else {
					in.Delim('[')
					if out.SavedSends == nil {
						if !in.IsDelim(']') {
							out.SavedSends = make([]WSSavedSend, 0, 1)
						} else {
							out.SavedSends = []WSSavedSend{}
						}
					} else {
						out.SavedSends = (out.SavedSends)[:0]
					}
					for !in.IsDelim(']') {
						var v4 WSSavedSend
						if in.IsNull() {
							in.Skip()
						} else {
							(v4).UnmarshalEasyJSON(in)
						}
						out.SavedSends = append(out.SavedSends, v4)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "split_ratio":
				if in.IsNull() {
					in.Skip()
				} else {
					out.SplitRatio = float32(in.Float32())
				}
			case "composer_ratio":
				if in.IsNull() {
					in.Skip()
				} else {
					out.ComposerRatio = float32(in.Float32())
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
func easyjsonBd887cf1EncodeTractoInternalPersist(out *jwriter.Writer, in WSTabState) {
	out.RawByte('{')
	first := true
	_ = first
	if len(in.Subprotocols) != 0 {
		const prefix string = ",\"subprotocols\":"
		first = false
		out.RawString(prefix[1:])
		{
			out.RawByte('[')
			for v5, v6 := range in.Subprotocols {
				if v5 > 0 {
					out.RawByte(',')
				}
				out.String(string(v6))
			}
			out.RawByte(']')
		}
	}
	if in.OptionsExpanded {
		const prefix string = ",\"options_expanded\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.Bool(bool(in.OptionsExpanded))
	}
	if in.SubprotosAbsHeight != 0 {
		const prefix string = ",\"subprotos_abs_height\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.Int(int(in.SubprotosAbsHeight))
	}
	if in.OfferDeflate {
		const prefix string = ",\"offer_deflate\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.Bool(bool(in.OfferDeflate))
	}
	if in.UseMsgpackProto {
		const prefix string = ",\"use_msgpack_proto\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.Bool(bool(in.UseMsgpackProto))
	}
	if in.ProtoCmd != "" {
		const prefix string = ",\"proto_cmd\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.ProtoCmd))
	}
	if in.ProtoSeq != "" {
		const prefix string = ",\"proto_seq\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.ProtoSeq))
	}
	if in.ProtoOpcode != "" {
		const prefix string = ",\"proto_opcode\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.ProtoOpcode))
	}
	if in.InsecureSkipVerify {
		const prefix string = ",\"insecure_skip_verify\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.Bool(bool(in.InsecureSkipVerify))
	}
	if in.UseTractoCA {
		const prefix string = ",\"use_tracto_ca\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.Bool(bool(in.UseTractoCA))
	}
	if len(in.SavedSends) != 0 {
		const prefix string = ",\"saved_sends\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		{
			out.RawByte('[')
			for v7, v8 := range in.SavedSends {
				if v7 > 0 {
					out.RawByte(',')
				}
				(v8).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	if in.SplitRatio != 0 {
		const prefix string = ",\"split_ratio\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.Float32(float32(in.SplitRatio))
	}
	if in.ComposerRatio != 0 {
		const prefix string = ",\"composer_ratio\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.Float32(float32(in.ComposerRatio))
	}
	out.RawByte('}')
}

func (v WSTabState) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonBd887cf1EncodeTractoInternalPersist(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v WSTabState) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonBd887cf1EncodeTractoInternalPersist(w, v)
}

func (v *WSTabState) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonBd887cf1DecodeTractoInternalPersist(&r, v)
	return r.Error()
}

func (v *WSTabState) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonBd887cf1DecodeTractoInternalPersist(l, v)
}
func easyjsonBd887cf1DecodeTractoInternalPersist1(in *jlexer.Lexer, out *WSSavedSend) {
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
		case "opcode":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Opcode = string(in.String())
			}
		case "text":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Text = string(in.String())
			}
		default:
			switch strings.ToLower(key) {
			case "name":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Name = string(in.String())
				}
			case "opcode":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Opcode = string(in.String())
				}
			case "text":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Text = string(in.String())
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
func easyjsonBd887cf1EncodeTractoInternalPersist1(out *jwriter.Writer, in WSSavedSend) {
	out.RawByte('{')
	first := true
	_ = first
	if in.Name != "" {
		const prefix string = ",\"name\":"
		first = false
		out.RawString(prefix[1:])
		out.String(string(in.Name))
	}
	if in.Opcode != "" {
		const prefix string = ",\"opcode\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Opcode))
	}
	if in.Text != "" {
		const prefix string = ",\"text\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Text))
	}
	out.RawByte('}')
}

func (v WSSavedSend) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonBd887cf1EncodeTractoInternalPersist1(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v WSSavedSend) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonBd887cf1EncodeTractoInternalPersist1(w, v)
}

func (v *WSSavedSend) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonBd887cf1DecodeTractoInternalPersist1(&r, v)
	return r.Error()
}

func (v *WSSavedSend) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonBd887cf1DecodeTractoInternalPersist1(l, v)
}
func easyjsonBd887cf1DecodeTractoInternalPersist2(in *jlexer.Lexer, out *TabState) {
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
		case "kind":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Kind = string(in.String())
			}
		case "title":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Title = string(in.String())
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
				out.Headers = nil
			} else {
				in.Delim('[')
				if out.Headers == nil {
					if !in.IsDelim(']') {
						out.Headers = make([]HeaderState, 0, 2)
					} else {
						out.Headers = []HeaderState{}
					}
				} else {
					out.Headers = (out.Headers)[:0]
				}
				for !in.IsDelim(']') {
					var v9 HeaderState
					if in.IsNull() {
						in.Skip()
					} else {
						(v9).UnmarshalEasyJSON(in)
					}
					out.Headers = append(out.Headers, v9)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "headers_expanded":
			if in.IsNull() {
				in.Skip()
			} else {
				out.HeadersExpanded = bool(in.Bool())
			}
		case "headers_abs_height":
			if in.IsNull() {
				in.Skip()
			} else {
				out.HeadersAbsHeight = int(in.Int())
			}
		case "split_ratio":
			if in.IsNull() {
				in.Skip()
			} else {
				out.SplitRatio = float32(in.Float32())
			}
		case "vstack_ratio":
			if in.IsNull() {
				in.Skip()
			} else {
				out.VStackRatio = float32(in.Float32())
			}
		case "layout_mode":
			if in.IsNull() {
				in.Skip()
			} else {
				out.LayoutMode = int(in.Int())
			}
		case "header_split_ratio":
			if in.IsNull() {
				in.Skip()
			} else {
				out.HeaderSplitRatio = float32(in.Float32())
			}
		case "req_wrap_enabled":
			if in.IsNull() {
				in.Skip()
				out.ReqWrapEnabled = nil
			} else {
				if out.ReqWrapEnabled == nil {
					out.ReqWrapEnabled = new(bool)
				}
				if in.IsNull() {
					in.Skip()
				} else {
					*out.ReqWrapEnabled = bool(in.Bool())
				}
			}
		case "collection_id":
			if in.IsNull() {
				in.Skip()
			} else {
				out.CollectionID = string(in.String())
			}
		case "node_path":
			if in.IsNull() {
				in.Skip()
				out.NodePath = nil
			} else {
				in.Delim('[')
				if out.NodePath == nil {
					if !in.IsDelim(']') {
						out.NodePath = make([]int, 0, 8)
					} else {
						out.NodePath = []int{}
					}
				} else {
					out.NodePath = (out.NodePath)[:0]
				}
				for !in.IsDelim(']') {
					var v10 int
					if in.IsNull() {
						in.Skip()
					} else {
						v10 = int(in.Int())
					}
					out.NodePath = append(out.NodePath, v10)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "body_type":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BodyType = string(in.String())
			}
		case "form_parts":
			if in.IsNull() {
				in.Skip()
				out.FormParts = nil
			} else {
				in.Delim('[')
				if out.FormParts == nil {
					if !in.IsDelim(']') {
						out.FormParts = make([]FormPartState, 0, 1)
					} else {
						out.FormParts = []FormPartState{}
					}
				} else {
					out.FormParts = (out.FormParts)[:0]
				}
				for !in.IsDelim(']') {
					var v11 FormPartState
					if in.IsNull() {
						in.Skip()
					} else {
						(v11).UnmarshalEasyJSON(in)
					}
					out.FormParts = append(out.FormParts, v11)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "url_encoded":
			if in.IsNull() {
				in.Skip()
				out.URLEncoded = nil
			} else {
				in.Delim('[')
				if out.URLEncoded == nil {
					if !in.IsDelim(']') {
						out.URLEncoded = make([]HeaderState, 0, 2)
					} else {
						out.URLEncoded = []HeaderState{}
					}
				} else {
					out.URLEncoded = (out.URLEncoded)[:0]
				}
				for !in.IsDelim(']') {
					var v12 HeaderState
					if in.IsNull() {
						in.Skip()
					} else {
						(v12).UnmarshalEasyJSON(in)
					}
					out.URLEncoded = append(out.URLEncoded, v12)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "binary_path":
			if in.IsNull() {
				in.Skip()
			} else {
				out.BinaryPath = string(in.String())
			}
		case "ws":
			if in.IsNull() {
				in.Skip()
				out.WS = nil
			} else {
				if out.WS == nil {
					out.WS = new(WSTabState)
				}
				if in.IsNull() {
					in.Skip()
				} else {
					(*out.WS).UnmarshalEasyJSON(in)
				}
			}
		case "gql":
			if in.IsNull() {
				in.Skip()
				out.GQL = nil
			} else {
				if out.GQL == nil {
					out.GQL = new(GQLTabState)
				}
				if in.IsNull() {
					in.Skip()
				} else {
					(*out.GQL).UnmarshalEasyJSON(in)
				}
			}
		default:
			switch strings.ToLower(key) {
			case "kind":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Kind = string(in.String())
				}
			case "title":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Title = string(in.String())
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
					out.Headers = nil
				} else {
					in.Delim('[')
					if out.Headers == nil {
						if !in.IsDelim(']') {
							out.Headers = make([]HeaderState, 0, 2)
						} else {
							out.Headers = []HeaderState{}
						}
					} else {
						out.Headers = (out.Headers)[:0]
					}
					for !in.IsDelim(']') {
						var v13 HeaderState
						if in.IsNull() {
							in.Skip()
						} else {
							(v13).UnmarshalEasyJSON(in)
						}
						out.Headers = append(out.Headers, v13)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "headers_expanded":
				if in.IsNull() {
					in.Skip()
				} else {
					out.HeadersExpanded = bool(in.Bool())
				}
			case "headers_abs_height":
				if in.IsNull() {
					in.Skip()
				} else {
					out.HeadersAbsHeight = int(in.Int())
				}
			case "split_ratio":
				if in.IsNull() {
					in.Skip()
				} else {
					out.SplitRatio = float32(in.Float32())
				}
			case "vstack_ratio":
				if in.IsNull() {
					in.Skip()
				} else {
					out.VStackRatio = float32(in.Float32())
				}
			case "layout_mode":
				if in.IsNull() {
					in.Skip()
				} else {
					out.LayoutMode = int(in.Int())
				}
			case "header_split_ratio":
				if in.IsNull() {
					in.Skip()
				} else {
					out.HeaderSplitRatio = float32(in.Float32())
				}
			case "req_wrap_enabled":
				if in.IsNull() {
					in.Skip()
					out.ReqWrapEnabled = nil
				} else {
					if out.ReqWrapEnabled == nil {
						out.ReqWrapEnabled = new(bool)
					}
					if in.IsNull() {
						in.Skip()
					} else {
						*out.ReqWrapEnabled = bool(in.Bool())
					}
				}
			case "collection_id":
				if in.IsNull() {
					in.Skip()
				} else {
					out.CollectionID = string(in.String())
				}
			case "node_path":
				if in.IsNull() {
					in.Skip()
					out.NodePath = nil
				} else {
					in.Delim('[')
					if out.NodePath == nil {
						if !in.IsDelim(']') {
							out.NodePath = make([]int, 0, 8)
						} else {
							out.NodePath = []int{}
						}
					} else {
						out.NodePath = (out.NodePath)[:0]
					}
					for !in.IsDelim(']') {
						var v14 int
						if in.IsNull() {
							in.Skip()
						} else {
							v14 = int(in.Int())
						}
						out.NodePath = append(out.NodePath, v14)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "body_type":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BodyType = string(in.String())
				}
			case "form_parts":
				if in.IsNull() {
					in.Skip()
					out.FormParts = nil
				} else {
					in.Delim('[')
					if out.FormParts == nil {
						if !in.IsDelim(']') {
							out.FormParts = make([]FormPartState, 0, 1)
						} else {
							out.FormParts = []FormPartState{}
						}
					} else {
						out.FormParts = (out.FormParts)[:0]
					}
					for !in.IsDelim(']') {
						var v15 FormPartState
						if in.IsNull() {
							in.Skip()
						} else {
							(v15).UnmarshalEasyJSON(in)
						}
						out.FormParts = append(out.FormParts, v15)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "url_encoded":
				if in.IsNull() {
					in.Skip()
					out.URLEncoded = nil
				} else {
					in.Delim('[')
					if out.URLEncoded == nil {
						if !in.IsDelim(']') {
							out.URLEncoded = make([]HeaderState, 0, 2)
						} else {
							out.URLEncoded = []HeaderState{}
						}
					} else {
						out.URLEncoded = (out.URLEncoded)[:0]
					}
					for !in.IsDelim(']') {
						var v16 HeaderState
						if in.IsNull() {
							in.Skip()
						} else {
							(v16).UnmarshalEasyJSON(in)
						}
						out.URLEncoded = append(out.URLEncoded, v16)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "binary_path":
				if in.IsNull() {
					in.Skip()
				} else {
					out.BinaryPath = string(in.String())
				}
			case "ws":
				if in.IsNull() {
					in.Skip()
					out.WS = nil
				} else {
					if out.WS == nil {
						out.WS = new(WSTabState)
					}
					if in.IsNull() {
						in.Skip()
					} else {
						(*out.WS).UnmarshalEasyJSON(in)
					}
				}
			case "gql":
				if in.IsNull() {
					in.Skip()
					out.GQL = nil
				} else {
					if out.GQL == nil {
						out.GQL = new(GQLTabState)
					}
					if in.IsNull() {
						in.Skip()
					} else {
						(*out.GQL).UnmarshalEasyJSON(in)
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
func easyjsonBd887cf1EncodeTractoInternalPersist2(out *jwriter.Writer, in TabState) {
	out.RawByte('{')
	first := true
	_ = first
	if in.Kind != "" {
		const prefix string = ",\"kind\":"
		first = false
		out.RawString(prefix[1:])
		out.String(string(in.Kind))
	}
	{
		const prefix string = ",\"title\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Title))
	}
	{
		const prefix string = ",\"method\":"
		out.RawString(prefix)
		out.String(string(in.Method))
	}
	{
		const prefix string = ",\"url\":"
		out.RawString(prefix)
		out.String(string(in.URL))
	}
	{
		const prefix string = ",\"body\":"
		out.RawString(prefix)
		out.String(string(in.Body))
	}
	{
		const prefix string = ",\"headers\":"
		out.RawString(prefix)
		if in.Headers == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v17, v18 := range in.Headers {
				if v17 > 0 {
					out.RawByte(',')
				}
				(v18).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	if in.HeadersExpanded {
		const prefix string = ",\"headers_expanded\":"
		out.RawString(prefix)
		out.Bool(bool(in.HeadersExpanded))
	}
	if in.HeadersAbsHeight != 0 {
		const prefix string = ",\"headers_abs_height\":"
		out.RawString(prefix)
		out.Int(int(in.HeadersAbsHeight))
	}
	{
		const prefix string = ",\"split_ratio\":"
		out.RawString(prefix)
		out.Float32(float32(in.SplitRatio))
	}
	if in.VStackRatio != 0 {
		const prefix string = ",\"vstack_ratio\":"
		out.RawString(prefix)
		out.Float32(float32(in.VStackRatio))
	}
	if in.LayoutMode != 0 {
		const prefix string = ",\"layout_mode\":"
		out.RawString(prefix)
		out.Int(int(in.LayoutMode))
	}
	if in.HeaderSplitRatio != 0 {
		const prefix string = ",\"header_split_ratio\":"
		out.RawString(prefix)
		out.Float32(float32(in.HeaderSplitRatio))
	}
	if in.ReqWrapEnabled != nil {
		const prefix string = ",\"req_wrap_enabled\":"
		out.RawString(prefix)
		out.Bool(bool(*in.ReqWrapEnabled))
	}
	if in.CollectionID != "" {
		const prefix string = ",\"collection_id\":"
		out.RawString(prefix)
		out.String(string(in.CollectionID))
	}
	if len(in.NodePath) != 0 {
		const prefix string = ",\"node_path\":"
		out.RawString(prefix)
		{
			out.RawByte('[')
			for v19, v20 := range in.NodePath {
				if v19 > 0 {
					out.RawByte(',')
				}
				out.Int(int(v20))
			}
			out.RawByte(']')
		}
	}
	if in.BodyType != "" {
		const prefix string = ",\"body_type\":"
		out.RawString(prefix)
		out.String(string(in.BodyType))
	}
	if len(in.FormParts) != 0 {
		const prefix string = ",\"form_parts\":"
		out.RawString(prefix)
		{
			out.RawByte('[')
			for v21, v22 := range in.FormParts {
				if v21 > 0 {
					out.RawByte(',')
				}
				(v22).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	if len(in.URLEncoded) != 0 {
		const prefix string = ",\"url_encoded\":"
		out.RawString(prefix)
		{
			out.RawByte('[')
			for v23, v24 := range in.URLEncoded {
				if v23 > 0 {
					out.RawByte(',')
				}
				(v24).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	if in.BinaryPath != "" {
		const prefix string = ",\"binary_path\":"
		out.RawString(prefix)
		out.String(string(in.BinaryPath))
	}
	if in.WS != nil {
		const prefix string = ",\"ws\":"
		out.RawString(prefix)
		(*in.WS).MarshalEasyJSON(out)
	}
	if in.GQL != nil {
		const prefix string = ",\"gql\":"
		out.RawString(prefix)
		(*in.GQL).MarshalEasyJSON(out)
	}
	out.RawByte('}')
}

func (v TabState) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonBd887cf1EncodeTractoInternalPersist2(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v TabState) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonBd887cf1EncodeTractoInternalPersist2(w, v)
}

func (v *TabState) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonBd887cf1DecodeTractoInternalPersist2(&r, v)
	return r.Error()
}

func (v *TabState) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonBd887cf1DecodeTractoInternalPersist2(l, v)
}
func easyjsonBd887cf1DecodeTractoInternalPersist3(in *jlexer.Lexer, out *HeaderState) {
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
func easyjsonBd887cf1EncodeTractoInternalPersist3(out *jwriter.Writer, in HeaderState) {
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

func (v HeaderState) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonBd887cf1EncodeTractoInternalPersist3(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v HeaderState) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonBd887cf1EncodeTractoInternalPersist3(w, v)
}

func (v *HeaderState) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonBd887cf1DecodeTractoInternalPersist3(&r, v)
	return r.Error()
}

func (v *HeaderState) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonBd887cf1DecodeTractoInternalPersist3(l, v)
}
func easyjsonBd887cf1DecodeTractoInternalPersist4(in *jlexer.Lexer, out *GQLTabState) {
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
		case "query":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Query = string(in.String())
			}
		case "variables":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Variables = string(in.String())
			}
		case "vars_split_ratio":
			if in.IsNull() {
				in.Skip()
			} else {
				out.VarsSplitRatio = float32(in.Float32())
			}
		default:
			switch strings.ToLower(key) {
			case "query":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Query = string(in.String())
				}
			case "variables":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Variables = string(in.String())
				}
			case "vars_split_ratio":
				if in.IsNull() {
					in.Skip()
				} else {
					out.VarsSplitRatio = float32(in.Float32())
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
func easyjsonBd887cf1EncodeTractoInternalPersist4(out *jwriter.Writer, in GQLTabState) {
	out.RawByte('{')
	first := true
	_ = first
	if in.Query != "" {
		const prefix string = ",\"query\":"
		first = false
		out.RawString(prefix[1:])
		out.String(string(in.Query))
	}
	if in.Variables != "" {
		const prefix string = ",\"variables\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.String(string(in.Variables))
	}
	if in.VarsSplitRatio != 0 {
		const prefix string = ",\"vars_split_ratio\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.Float32(float32(in.VarsSplitRatio))
	}
	out.RawByte('}')
}

func (v GQLTabState) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonBd887cf1EncodeTractoInternalPersist4(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v GQLTabState) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonBd887cf1EncodeTractoInternalPersist4(w, v)
}

func (v *GQLTabState) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonBd887cf1DecodeTractoInternalPersist4(&r, v)
	return r.Error()
}

func (v *GQLTabState) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonBd887cf1DecodeTractoInternalPersist4(l, v)
}
func easyjsonBd887cf1DecodeTractoInternalPersist5(in *jlexer.Lexer, out *FormPartState) {
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
		case "kind":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Kind = string(in.String())
			}
		case "value":
			if in.IsNull() {
				in.Skip()
			} else {
				out.Value = string(in.String())
			}
		case "file_path":
			if in.IsNull() {
				in.Skip()
			} else {
				out.FilePath = string(in.String())
			}
		default:
			switch strings.ToLower(key) {
			case "key":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Key = string(in.String())
				}
			case "kind":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Kind = string(in.String())
				}
			case "value":
				if in.IsNull() {
					in.Skip()
				} else {
					out.Value = string(in.String())
				}
			case "file_path":
				if in.IsNull() {
					in.Skip()
				} else {
					out.FilePath = string(in.String())
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
func easyjsonBd887cf1EncodeTractoInternalPersist5(out *jwriter.Writer, in FormPartState) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"key\":"
		out.RawString(prefix[1:])
		out.String(string(in.Key))
	}
	{
		const prefix string = ",\"kind\":"
		out.RawString(prefix)
		out.String(string(in.Kind))
	}
	if in.Value != "" {
		const prefix string = ",\"value\":"
		out.RawString(prefix)
		out.String(string(in.Value))
	}
	if in.FilePath != "" {
		const prefix string = ",\"file_path\":"
		out.RawString(prefix)
		out.String(string(in.FilePath))
	}
	out.RawByte('}')
}

func (v FormPartState) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonBd887cf1EncodeTractoInternalPersist5(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v FormPartState) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonBd887cf1EncodeTractoInternalPersist5(w, v)
}

func (v *FormPartState) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonBd887cf1DecodeTractoInternalPersist5(&r, v)
	return r.Error()
}

func (v *FormPartState) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonBd887cf1DecodeTractoInternalPersist5(l, v)
}
func easyjsonBd887cf1DecodeTractoInternalPersist6(in *jlexer.Lexer, out *AppState) {
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
		case "tabs":
			if in.IsNull() {
				in.Skip()
				out.Tabs = nil
			} else {
				in.Delim('[')
				if out.Tabs == nil {
					if !in.IsDelim(']') {
						out.Tabs = make([]TabState, 0, 0)
					} else {
						out.Tabs = []TabState{}
					}
				} else {
					out.Tabs = (out.Tabs)[:0]
				}
				for !in.IsDelim(']') {
					var v25 TabState
					if in.IsNull() {
						in.Skip()
					} else {
						(v25).UnmarshalEasyJSON(in)
					}
					out.Tabs = append(out.Tabs, v25)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "active_idx":
			if in.IsNull() {
				in.Skip()
			} else {
				out.ActiveIdx = int(in.Int())
			}
		case "active_env_id":
			if in.IsNull() {
				in.Skip()
			} else {
				out.ActiveEnvID = string(in.String())
			}
		case "sidebar_width_px":
			if in.IsNull() {
				in.Skip()
			} else {
				out.SidebarWidthPx = int(in.Int())
			}
		case "sidebar_env_height_px":
			if in.IsNull() {
				in.Skip()
			} else {
				out.SidebarEnvHeightPx = int(in.Int())
			}
		case "settings":
			if in.IsNull() {
				in.Skip()
				out.Settings = nil
			} else {
				if out.Settings == nil {
					out.Settings = new(model.AppSettings)
				}
				if in.IsNull() {
					in.Skip()
				} else {
					(*out.Settings).UnmarshalEasyJSON(in)
				}
			}
		case "env_ids_order":
			if in.IsNull() {
				in.Skip()
				out.EnvIDsOrder = nil
			} else {
				in.Delim('[')
				if out.EnvIDsOrder == nil {
					if !in.IsDelim(']') {
						out.EnvIDsOrder = make([]string, 0, 4)
					} else {
						out.EnvIDsOrder = []string{}
					}
				} else {
					out.EnvIDsOrder = (out.EnvIDsOrder)[:0]
				}
				for !in.IsDelim(']') {
					var v26 string
					if in.IsNull() {
						in.Skip()
					} else {
						v26 = string(in.String())
					}
					out.EnvIDsOrder = append(out.EnvIDsOrder, v26)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "collection_ids_order":
			if in.IsNull() {
				in.Skip()
				out.CollectionIDsOrder = nil
			} else {
				in.Delim('[')
				if out.CollectionIDsOrder == nil {
					if !in.IsDelim(']') {
						out.CollectionIDsOrder = make([]string, 0, 4)
					} else {
						out.CollectionIDsOrder = []string{}
					}
				} else {
					out.CollectionIDsOrder = (out.CollectionIDsOrder)[:0]
				}
				for !in.IsDelim(']') {
					var v27 string
					if in.IsNull() {
						in.Skip()
					} else {
						v27 = string(in.String())
					}
					out.CollectionIDsOrder = append(out.CollectionIDsOrder, v27)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "sidebar_section":
			if in.IsNull() {
				in.Skip()
			} else {
				out.SidebarSection = string(in.String())
			}
		case "sidebar_scripts_height_px":
			if in.IsNull() {
				in.Skip()
			} else {
				out.SidebarScriptsHeightPx = int(in.Int())
			}
		case "collection_expanded":
			if in.IsNull() {
				in.Skip()
			} else {
				in.Delim('{')
				if !in.IsDelim('}') {
					out.CollectionExpanded = make(map[string][][]int)
				} else {
					out.CollectionExpanded = nil
				}
				for !in.IsDelim('}') {
					key := string(in.String())
					in.WantColon()
					var v28 [][]int
					if in.IsNull() {
						in.Skip()
						v28 = nil
					} else {
						in.Delim('[')
						if v28 == nil {
							if !in.IsDelim(']') {
								v28 = make([][]int, 0, 2)
							} else {
								v28 = [][]int{}
							}
						} else {
							v28 = (v28)[:0]
						}
						for !in.IsDelim(']') {
							var v29 []int
							if in.IsNull() {
								in.Skip()
								v29 = nil
							} else {
								in.Delim('[')
								if v29 == nil {
									if !in.IsDelim(']') {
										v29 = make([]int, 0, 8)
									} else {
										v29 = []int{}
									}
								} else {
									v29 = (v29)[:0]
								}
								for !in.IsDelim(']') {
									var v30 int
									if in.IsNull() {
										in.Skip()
									} else {
										v30 = int(in.Int())
									}
									v29 = append(v29, v30)
									in.WantComma()
								}
								in.Delim(']')
							}
							v28 = append(v28, v29)
							in.WantComma()
						}
						in.Delim(']')
					}
					(out.CollectionExpanded)[key] = v28
					in.WantComma()
				}
				in.Delim('}')
			}
		case "cols_expanded":
			if in.IsNull() {
				in.Skip()
				out.ColsExpanded = nil
			} else {
				if out.ColsExpanded == nil {
					out.ColsExpanded = new(bool)
				}
				if in.IsNull() {
					in.Skip()
				} else {
					*out.ColsExpanded = bool(in.Bool())
				}
			}
		case "envs_expanded":
			if in.IsNull() {
				in.Skip()
				out.EnvsExpanded = nil
			} else {
				if out.EnvsExpanded == nil {
					out.EnvsExpanded = new(bool)
				}
				if in.IsNull() {
					in.Skip()
				} else {
					*out.EnvsExpanded = bool(in.Bool())
				}
			}
		case "scripts_expanded":
			if in.IsNull() {
				in.Skip()
				out.ScriptsExpanded = nil
			} else {
				if out.ScriptsExpanded == nil {
					out.ScriptsExpanded = new(bool)
				}
				if in.IsNull() {
					in.Skip()
				} else {
					*out.ScriptsExpanded = bool(in.Bool())
				}
			}
		case "window_width_dp":
			if in.IsNull() {
				in.Skip()
			} else {
				out.WindowWidthDp = int(in.Int())
			}
		case "window_height_dp":
			if in.IsNull() {
				in.Skip()
			} else {
				out.WindowHeightDp = int(in.Int())
			}
		case "window_mode":
			if in.IsNull() {
				in.Skip()
			} else {
				out.WindowMode = string(in.String())
			}
		default:
			switch strings.ToLower(key) {
			case "tabs":
				if in.IsNull() {
					in.Skip()
					out.Tabs = nil
				} else {
					in.Delim('[')
					if out.Tabs == nil {
						if !in.IsDelim(']') {
							out.Tabs = make([]TabState, 0, 0)
						} else {
							out.Tabs = []TabState{}
						}
					} else {
						out.Tabs = (out.Tabs)[:0]
					}
					for !in.IsDelim(']') {
						var v31 TabState
						if in.IsNull() {
							in.Skip()
						} else {
							(v31).UnmarshalEasyJSON(in)
						}
						out.Tabs = append(out.Tabs, v31)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "active_idx":
				if in.IsNull() {
					in.Skip()
				} else {
					out.ActiveIdx = int(in.Int())
				}
			case "active_env_id":
				if in.IsNull() {
					in.Skip()
				} else {
					out.ActiveEnvID = string(in.String())
				}
			case "sidebar_width_px":
				if in.IsNull() {
					in.Skip()
				} else {
					out.SidebarWidthPx = int(in.Int())
				}
			case "sidebar_env_height_px":
				if in.IsNull() {
					in.Skip()
				} else {
					out.SidebarEnvHeightPx = int(in.Int())
				}
			case "settings":
				if in.IsNull() {
					in.Skip()
					out.Settings = nil
				} else {
					if out.Settings == nil {
						out.Settings = new(model.AppSettings)
					}
					if in.IsNull() {
						in.Skip()
					} else {
						(*out.Settings).UnmarshalEasyJSON(in)
					}
				}
			case "env_ids_order":
				if in.IsNull() {
					in.Skip()
					out.EnvIDsOrder = nil
				} else {
					in.Delim('[')
					if out.EnvIDsOrder == nil {
						if !in.IsDelim(']') {
							out.EnvIDsOrder = make([]string, 0, 4)
						} else {
							out.EnvIDsOrder = []string{}
						}
					} else {
						out.EnvIDsOrder = (out.EnvIDsOrder)[:0]
					}
					for !in.IsDelim(']') {
						var v32 string
						if in.IsNull() {
							in.Skip()
						} else {
							v32 = string(in.String())
						}
						out.EnvIDsOrder = append(out.EnvIDsOrder, v32)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "collection_ids_order":
				if in.IsNull() {
					in.Skip()
					out.CollectionIDsOrder = nil
				} else {
					in.Delim('[')
					if out.CollectionIDsOrder == nil {
						if !in.IsDelim(']') {
							out.CollectionIDsOrder = make([]string, 0, 4)
						} else {
							out.CollectionIDsOrder = []string{}
						}
					} else {
						out.CollectionIDsOrder = (out.CollectionIDsOrder)[:0]
					}
					for !in.IsDelim(']') {
						var v33 string
						if in.IsNull() {
							in.Skip()
						} else {
							v33 = string(in.String())
						}
						out.CollectionIDsOrder = append(out.CollectionIDsOrder, v33)
						in.WantComma()
					}
					in.Delim(']')
				}
			case "sidebar_section":
				if in.IsNull() {
					in.Skip()
				} else {
					out.SidebarSection = string(in.String())
				}
			case "sidebar_scripts_height_px":
				if in.IsNull() {
					in.Skip()
				} else {
					out.SidebarScriptsHeightPx = int(in.Int())
				}
			case "collection_expanded":
				if in.IsNull() {
					in.Skip()
				} else {
					in.Delim('{')
					if !in.IsDelim('}') {
						out.CollectionExpanded = make(map[string][][]int)
					} else {
						out.CollectionExpanded = nil
					}
					for !in.IsDelim('}') {
						key := string(in.String())
						in.WantColon()
						var v34 [][]int
						if in.IsNull() {
							in.Skip()
							v34 = nil
						} else {
							in.Delim('[')
							if v34 == nil {
								if !in.IsDelim(']') {
									v34 = make([][]int, 0, 2)
								} else {
									v34 = [][]int{}
								}
							} else {
								v34 = (v34)[:0]
							}
							for !in.IsDelim(']') {
								var v35 []int
								if in.IsNull() {
									in.Skip()
									v35 = nil
								} else {
									in.Delim('[')
									if v35 == nil {
										if !in.IsDelim(']') {
											v35 = make([]int, 0, 8)
										} else {
											v35 = []int{}
										}
									} else {
										v35 = (v35)[:0]
									}
									for !in.IsDelim(']') {
										var v36 int
										if in.IsNull() {
											in.Skip()
										} else {
											v36 = int(in.Int())
										}
										v35 = append(v35, v36)
										in.WantComma()
									}
									in.Delim(']')
								}
								v34 = append(v34, v35)
								in.WantComma()
							}
							in.Delim(']')
						}
						(out.CollectionExpanded)[key] = v34
						in.WantComma()
					}
					in.Delim('}')
				}
			case "cols_expanded":
				if in.IsNull() {
					in.Skip()
					out.ColsExpanded = nil
				} else {
					if out.ColsExpanded == nil {
						out.ColsExpanded = new(bool)
					}
					if in.IsNull() {
						in.Skip()
					} else {
						*out.ColsExpanded = bool(in.Bool())
					}
				}
			case "envs_expanded":
				if in.IsNull() {
					in.Skip()
					out.EnvsExpanded = nil
				} else {
					if out.EnvsExpanded == nil {
						out.EnvsExpanded = new(bool)
					}
					if in.IsNull() {
						in.Skip()
					} else {
						*out.EnvsExpanded = bool(in.Bool())
					}
				}
			case "scripts_expanded":
				if in.IsNull() {
					in.Skip()
					out.ScriptsExpanded = nil
				} else {
					if out.ScriptsExpanded == nil {
						out.ScriptsExpanded = new(bool)
					}
					if in.IsNull() {
						in.Skip()
					} else {
						*out.ScriptsExpanded = bool(in.Bool())
					}
				}
			case "window_width_dp":
				if in.IsNull() {
					in.Skip()
				} else {
					out.WindowWidthDp = int(in.Int())
				}
			case "window_height_dp":
				if in.IsNull() {
					in.Skip()
				} else {
					out.WindowHeightDp = int(in.Int())
				}
			case "window_mode":
				if in.IsNull() {
					in.Skip()
				} else {
					out.WindowMode = string(in.String())
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
func easyjsonBd887cf1EncodeTractoInternalPersist6(out *jwriter.Writer, in AppState) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"tabs\":"
		out.RawString(prefix[1:])
		if in.Tabs == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v37, v38 := range in.Tabs {
				if v37 > 0 {
					out.RawByte(',')
				}
				(v38).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	{
		const prefix string = ",\"active_idx\":"
		out.RawString(prefix)
		out.Int(int(in.ActiveIdx))
	}
	{
		const prefix string = ",\"active_env_id\":"
		out.RawString(prefix)
		out.String(string(in.ActiveEnvID))
	}
	{
		const prefix string = ",\"sidebar_width_px\":"
		out.RawString(prefix)
		out.Int(int(in.SidebarWidthPx))
	}
	{
		const prefix string = ",\"sidebar_env_height_px\":"
		out.RawString(prefix)
		out.Int(int(in.SidebarEnvHeightPx))
	}
	if in.Settings != nil {
		const prefix string = ",\"settings\":"
		out.RawString(prefix)
		(*in.Settings).MarshalEasyJSON(out)
	}
	if len(in.EnvIDsOrder) != 0 {
		const prefix string = ",\"env_ids_order\":"
		out.RawString(prefix)
		{
			out.RawByte('[')
			for v39, v40 := range in.EnvIDsOrder {
				if v39 > 0 {
					out.RawByte(',')
				}
				out.String(string(v40))
			}
			out.RawByte(']')
		}
	}
	if len(in.CollectionIDsOrder) != 0 {
		const prefix string = ",\"collection_ids_order\":"
		out.RawString(prefix)
		{
			out.RawByte('[')
			for v41, v42 := range in.CollectionIDsOrder {
				if v41 > 0 {
					out.RawByte(',')
				}
				out.String(string(v42))
			}
			out.RawByte(']')
		}
	}
	if in.SidebarSection != "" {
		const prefix string = ",\"sidebar_section\":"
		out.RawString(prefix)
		out.String(string(in.SidebarSection))
	}
	if in.SidebarScriptsHeightPx != 0 {
		const prefix string = ",\"sidebar_scripts_height_px\":"
		out.RawString(prefix)
		out.Int(int(in.SidebarScriptsHeightPx))
	}
	if len(in.CollectionExpanded) != 0 {
		const prefix string = ",\"collection_expanded\":"
		out.RawString(prefix)
		{
			out.RawByte('{')
			v43First := true
			for v43Name, v43Value := range in.CollectionExpanded {
				if v43First {
					v43First = false
				} else {
					out.RawByte(',')
				}
				out.String(string(v43Name))
				out.RawByte(':')
				if v43Value == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
					out.RawString("null")
				} else {
					out.RawByte('[')
					for v44, v45 := range v43Value {
						if v44 > 0 {
							out.RawByte(',')
						}
						if v45 == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
							out.RawString("null")
						} else {
							out.RawByte('[')
							for v46, v47 := range v45 {
								if v46 > 0 {
									out.RawByte(',')
								}
								out.Int(int(v47))
							}
							out.RawByte(']')
						}
					}
					out.RawByte(']')
				}
			}
			out.RawByte('}')
		}
	}
	if in.ColsExpanded != nil {
		const prefix string = ",\"cols_expanded\":"
		out.RawString(prefix)
		out.Bool(bool(*in.ColsExpanded))
	}
	if in.EnvsExpanded != nil {
		const prefix string = ",\"envs_expanded\":"
		out.RawString(prefix)
		out.Bool(bool(*in.EnvsExpanded))
	}
	if in.ScriptsExpanded != nil {
		const prefix string = ",\"scripts_expanded\":"
		out.RawString(prefix)
		out.Bool(bool(*in.ScriptsExpanded))
	}
	if in.WindowWidthDp != 0 {
		const prefix string = ",\"window_width_dp\":"
		out.RawString(prefix)
		out.Int(int(in.WindowWidthDp))
	}
	if in.WindowHeightDp != 0 {
		const prefix string = ",\"window_height_dp\":"
		out.RawString(prefix)
		out.Int(int(in.WindowHeightDp))
	}
	if in.WindowMode != "" {
		const prefix string = ",\"window_mode\":"
		out.RawString(prefix)
		out.String(string(in.WindowMode))
	}
	out.RawByte('}')
}

func (v AppState) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjsonBd887cf1EncodeTractoInternalPersist6(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

func (v AppState) MarshalEasyJSON(w *jwriter.Writer) {
	easyjsonBd887cf1EncodeTractoInternalPersist6(w, v)
}

func (v *AppState) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjsonBd887cf1DecodeTractoInternalPersist6(&r, v)
	return r.Error()
}

func (v *AppState) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjsonBd887cf1DecodeTractoInternalPersist6(l, v)
}
