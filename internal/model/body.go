package model

type BodyType uint8

const (
	BodyRaw BodyType = iota
	BodyNone
	BodyFormData
	BodyURLEncoded
	BodyBinary
)

func (b BodyType) String() string {
	switch b {
	case BodyNone:
		return "none"
	case BodyFormData:
		return "form-data"
	case BodyURLEncoded:
		return "x-www-form-urlencoded"
	case BodyBinary:
		return "binary"
	default:
		return "raw"
	}
}

func (b BodyType) PostmanMode() string {
	switch b {
	case BodyNone:
		return "none"
	case BodyFormData:
		return "formdata"
	case BodyURLEncoded:
		return "urlencoded"
	case BodyBinary:
		return "file"
	default:
		return "raw"
	}
}

func BodyTypeFromMode(s string) BodyType {
	switch s {
	case "none":
		return BodyNone
	case "formdata", "form-data":
		return BodyFormData
	case "urlencoded", "x-www-form-urlencoded":
		return BodyURLEncoded
	case "file", "binary":
		return BodyBinary
	default:
		return BodyRaw
	}
}

type FormPartKind uint8

const (
	FormPartText FormPartKind = iota
	FormPartFile
)
