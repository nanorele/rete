package flow

import "strings"

type TabFormPart struct {
	Key      string
	Value    string
	IsFile   bool
	FilePath string
}

type TabRequest struct {
	Name       string
	Kind       NodeKind
	Method     string
	URL        string
	Headers    [][2]string
	Body       string
	BodyType   string
	URLEncoded [][2]string
	FormParts  []TabFormPart
	BinaryPath string
	AuthType   string
	AuthToken  string
	AuthUser   string
	AuthPass   string
	Cookies    [][2]string
	GQLQuery   string
	GQLVars    string
	Subprotos  []string
	WSMessage  string
	WSOpcode   string
	WSInsecure bool
}

func (ed *Editor) AddRequestNode(req TabRequest) *Node {
	ed.pushHistory()
	c := ed.viewCenterWorld()
	off := float32(len(ed.Scenario.Nodes)%5) * 24
	kind := req.Kind
	if !kind.IsRequest() {
		kind = KindRequest
	}
	n := ed.newNodeAt(kind, c.X-ed.nodeW/2+off, c.Y-ed.nodeH/2+off)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = kind.Title()
	}
	n.NameEd.SetText(name)
	n.URLEd.SetText(req.URL)

	var hdr []string
	for _, h := range req.Headers {
		if h[0] == "" {
			continue
		}
		hdr = append(hdr, h[0]+": "+h[1])
	}
	n.HeadersEd.SetText(strings.Join(hdr, "\n"))

	var cookies []string
	for _, ck := range req.Cookies {
		if ck[0] == "" {
			continue
		}
		cookies = append(cookies, ck[0]+"="+ck[1])
	}
	n.CookiesEd.SetText(strings.Join(cookies, "\n"))

	if req.AuthType == "bearer" || req.AuthType == "basic" {
		n.AuthType = req.AuthType
		n.AuthTokenEd.SetText(req.AuthToken)
		n.AuthUserEd.SetText(req.AuthUser)
		n.AuthPassEd.SetText(req.AuthPass)
	}

	switch kind {
	case KindRequest:
		if req.Method != "" {
			n.Method = req.Method
		}
		bt := req.BodyType
		if bt == "" {
			bt = "raw"
		}
		n.BodyType = bt
		switch bt {
		case "urlencoded":
			var lines []string
			for _, kv := range req.URLEncoded {
				if kv[0] == "" {
					continue
				}
				lines = append(lines, kv[0]+"="+kv[1])
			}
			n.BodyEd.SetText(strings.Join(lines, "\n"))
		case "form":
			var lines []string
			for _, fp := range req.FormParts {
				if fp.Key == "" {
					continue
				}
				if fp.IsFile {
					lines = append(lines, fp.Key+"=@"+fp.FilePath)
				} else {
					lines = append(lines, fp.Key+"="+fp.Value)
				}
			}
			n.BodyEd.SetText(strings.Join(lines, "\n"))
		case "binary":
			n.BinPathEd.SetText(req.BinaryPath)
		default:
			n.BodyEd.SetText(req.Body)
		}
	case KindWSRequest:
		n.BodyEd.SetText(req.WSMessage)
		if req.WSOpcode != "" {
			n.WSOpcode = req.WSOpcode
		}
		n.InsecureTLS = req.WSInsecure
		n.SubprotosEd.SetText(strings.Join(req.Subprotos, ", "))
	case KindGQLRequest:
		n.BodyEd.SetText(req.GQLQuery)
		n.VarsEd.SetText(req.GQLVars)
	}

	ed.Scenario.Nodes = append(ed.Scenario.Nodes, n)
	ed.selectOnly(n.ID)
	ed.mode = modeProps
	ed.pendingFit = true
	ed.SaveScenario()
	return n
}
