package parser

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	hcl "github.com/openTaho/taho-go-hcl"
)

const (
	HclAlphaNumericDashOrUnderscore = iota
	HclEquals
	HclNewLine
	HclOther
	HclPound
	HclSpace
	HclUnknown
)

type HclThinDir struct {
	name   string
	parser hcl.HclParser
}

type HclThinNode struct {
	hcl.HclNode

	pair     hcl.HclNode
	operator string
	value    string
	hclType  hcl.HclType
	nodes    []hcl.HclNode
}

func (h *HclThinNode) Operator() string {
	return h.operator
}

func (h *HclThinNode) Nodes() []hcl.HclNode {
	if h.nodes == nil {
		return []hcl.HclNode{}
	}
	return h.nodes
}

func (h *HclThinNode) SetNodes(nodes []hcl.HclNode) {
	h.nodes = nodes
}

func (h *HclThinNode) Type() hcl.HclType {
	return h.hclType
}

type HclThinFile struct {
	name string
}

type HclThinParser struct{}

func (h *HclThinDir) Files() ([]hcl.HclFile, error) {
	files, err := os.ReadDir(h.name)
	if err != nil {
		return nil, err
	}

	hclFiles := []hcl.HclFile{}
	for _, file := range files {
		name := file.Name()
		if !file.IsDir() {
			m, err := regexp.MatchString(".*\\.(hcl|tf|tfvars)", name)
			if err != nil {
				panic(err)
			}
			if m {
				hclFile := h.parser.NewFile(filepath.Join(h.name, name))
				hclFiles = append(hclFiles, hclFile)
			}
		}
	}

	return hclFiles, err
}

func (h *HclThinDir) Name() string {
	return h.name
}

func (_ *HclThinDir) SetName(value string) {
	panic("not implemented")
}

func (_ *HclThinDir) Nodes() []hcl.HclNode {
	panic("not implemented")
}

func (p HclThinParser) NewDir(name string) hcl.HclDir {
	if !strings.HasPrefix(name, "/") {
		wd, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		name = filepath.Join(wd, name)
	}

	return &HclThinDir{
		name:   name,
		parser: p,
	}
}

func (HclThinParser) NewFile(name string) hcl.HclFile {
	if !strings.HasPrefix(name, "/") {
		wd, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		name = filepath.Join(wd, name)
	}

	return &HclThinFile{
		name: name,
	}
}

func (h *HclThinFile) Nodes() []hcl.HclNode {
	nodes := []hcl.HclNode{}

	f, err := os.Open(h.name)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	fileElement := &HclThinNode{
		value: filepath.Base(h.name),
	}
	fileElement.SetNodes(readNodes(f, err, fileElement.Nodes()))
	nodes = append(nodes, fileElement)

	return nodes
}

func readNodes(f *os.File, err error, nodes []hcl.HclNode) []hcl.HclNode {
	c := HclUnknown
	cc := HclAlphaNumericDashOrUnderscore
	var hclType hcl.HclType
	hclType = hcl.HclTypeOther
	hereDocExpected1 := false
	hereDocExpected2 := false
	ignore := false
	expression := false
	expressionPairing := false
	pairing := false
	pairingNext := false
	pairingPending1 := false
	pairingPending2 := false
	pairingSoon := false
	stringNext := false
	br := bufio.NewReader(f)

	var r rune
	var sb strings.Builder

	willBreak := false
	for {
		if stringNext {
			stringNext = false
			hclType = hcl.HclTypeString
			readString(br, &sb)
			ignore = true
			r = 0
		} else {
			r, _, err = br.ReadRune()
			if err != nil {
				r = ' '
				willBreak = true
			}
		}

		if hereDocExpected2 {
			hereDocExpected2 = false
			ignore = true
			sb.WriteRune(r)
			hclType = readHereDoc(br, &sb)
			if pairingNext {
				pairingNext = false
				eLen := len(nodes)
				newElement := &HclThinNode{
					hclType: hclType,
					pair:    nodes[eLen-5],
					value:   sb.String(),
				}
				nodes = pair(nodes, newElement, 3)
			} else {
				nodes = append(nodes, &HclThinNode{
					value:   sb.String(),
					hclType: hcl.HclTypeHereDoc,
				})
			}
			sb.Reset()
		} else if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_' {
			c = HclAlphaNumericDashOrUnderscore
		} else if r == '"' {
			stringNext = true
			c = HclOther
			cc = HclUnknown
		} else if r == '\n' {
			c = HclNewLine
			cc = HclUnknown
		} else if unicode.IsSpace(r) {
			c = HclSpace
		} else if r == '<' {
			c = HclOther
			cc = HclUnknown
			if hereDocExpected1 {
				hereDocExpected1 = false
				hereDocExpected2 = true
			} else if pairingPending2 {
				hereDocExpected1 = true
				pairingPending2 = false
				pairingPending1 = true
			}
		} else if r == '+' {
			c = HclEquals
			cc = HclUnknown
			expression = !pairing
			expressionPairing = pairing
			pairingSoon = true
		} else if r == '=' {
			c = HclEquals
			cc = HclUnknown
			pairingSoon = true
		} else if r == '#' {
			c = HclPound
			cc = HclUnknown
			_, err = sb.WriteRune(r)
			if err != nil {
				panic(err)
			}
			hclType = hcl.HclTypeComment
			readComment(br, &sb)
			ignore = true
		} else {
			c = HclOther
			cc = HclUnknown
		}

		if cc != c {
			v := sb.String()
			if hclType == hcl.HclTypeString {
				v = strings.TrimPrefix(v, "\"")
				v = strings.TrimSuffix(v, "\"")
			}
			if v != "" {
				if pairing {
					pairing = false
					if expressionPairing {
						expressionPairing = false
						pairingSoon = true
					} else {
						nodes, v = doPairing(nodes, v, expression)
						expression = expressionPairing
						expressionPairing = false
					}
				} else {
					nLen := len(nodes)
					lastWasComment := nLen > 0 && nodes[nLen-1].Type() == hcl.HclTypeComment
					if hclType == hcl.HclTypeComment && lastWasComment {
						v = nodes[len(nodes)-1].Value() + v
						nodes[len(nodes)-1].SetValue(v)
					} else {
						nodes = append(nodes, &HclThinNode{
							value:   v,
							hclType: hclType,
						})
						if pairingNext {
							pairingNext = false
							pairing = true
						}
						if pairingPending2 {
							pairingPending2 = false
							pairing = strings.TrimSpace(v) != "" && strings.TrimSpace(v) != "<"
							pairingNext = strings.TrimSpace(v) == "" || strings.TrimSpace(v) == "<"
						}
						if pairingPending1 {
							pairingPending1 = false
							pairingPending2 = true
						}
						if pairingSoon {
							pairingSoon = false
							pairingPending1 = true
						}
					}
				}
				sb.Reset()
			}
			cc = c
		}

		hclType = hcl.HclTypeToken

		if willBreak {
			break
		} else if !ignore {
			sb.WriteRune(r)
		} else {
			ignore = false
		}
	}

	return nodes
}

func doPairing(nodes []hcl.HclNode, v string, expression bool) ([]hcl.HclNode, string) {
	nLen := len(nodes)
	last := nLen - 1

	n0 := nodes[last-0]
	n1 := nodes[last-1]
	n2 := nodes[last-2]
	n3 := nodes[last-3]
	n4 := nodes[last-4]

	n0v := n0.Value()
	n1v := n1.Value()
	n2v := n2.Value()
	n3v := n3.Value()

	operator := "="
	if expression {
		operator = "+"
	}
	var newElement hcl.HclNode
	if operator == n2v &&
		"" == strings.TrimSpace(n1v) &&
		"" == strings.TrimSpace(n3v) {
		v = n0v
		newElement = &HclThinNode{
			hclType:  n0.Type(),
			pair:     n4,
			operator: operator,
			value:    v,
		}
		nodes = pair(nodes, newElement, 3)
	} else if operator == n2v {
		v = n0v
		newElement = &HclThinNode{
			hclType:  n0.Type(),
			pair:     n3,
			operator: operator,
			value:    v,
		}
		nodes = pair(nodes, newElement, 2)
	} else if operator == n1v {
		v = n0v
		ep := n2
		trim := 1
		if strings.TrimSpace(n2v) == "" {
			ep = n3
			trim = 2
		}
		newElement = &HclThinNode{
			hclType:  n0.Type(),
			pair:     ep,
			operator: operator,
			value:    v,
		}
		nodes = pair(nodes, newElement, trim)
	} else {
		panic("2")
	}
	return nodes, v
}

func pair(nodes []hcl.HclNode, newNode hcl.HclNode, trim int) []hcl.HclNode {
	last := len(nodes) - 1
	nodes = nodes[:last-trim]
	var hclType hcl.HclType
	nType := newNode.Type()
	if nType == hcl.HclTypeHereDoc ||
		newNode.Type() == hcl.HclTypeHereDocWithIndent {
		hclType = hcl.HclTypeMultiLinePair
	} else {
		hclType = hcl.HclTypeSingleLinePair
	}
	nodes[len(nodes)-1] = &HclThinNode{
		hclType: hclType,
		pair:    newNode,
	}
	return nodes
}

func readHereDoc(br *bufio.Reader, sb *strings.Builder) hcl.HclType {
	s, err := br.ReadString('\n')
	if err != nil {
		panic(err)
	}
	sb.WriteString(s)
	end := strings.TrimPrefix(sb.String(), "<")
	var hclType hcl.HclType
	hclType = hcl.HclTypeHereDoc
	if strings.HasPrefix(end, "-") {
		hclType = hcl.HclTypeHereDocWithIndent
		end = strings.TrimPrefix(end, "-")
	}
	sb.Reset()
	for true {
		s, err = br.ReadString('\n')
		if strings.HasSuffix(s, end) {
			s = sb.String()
			s = strings.TrimSuffix(s, "\n")
			sb.Reset()
			sb.WriteString(s)
			break
		}
		sb.WriteString(s + "\n")
	}
	return hclType
}

func readComment(br *bufio.Reader, sb *strings.Builder) {
	s, err := br.ReadString('\n')
	if err != nil {
		panic(err)
	}
	sb.WriteString(s)
}

func readString(br *bufio.Reader, sb *strings.Builder) {
	s, err := br.ReadString('"')
	if err != nil {
		panic(err)
	}
	sb.WriteString(s)
	if strings.HasSuffix(s, "\\\"") {
		readString(br, sb)
	}
}

func (h *HclThinNode) SetValue(value string) {
	h.value = value
}

func (h *HclThinNode) Value() string {
	return h.value
}

func (h *HclThinNode) SetPair(pair hcl.HclNode) {
	h.pair = pair
}

func (h *HclThinNode) Pair() hcl.HclNode {
	return h.pair
}

func (h *HclThinFile) Name() string {
	return h.name
}

func (*HclThinFile) SetName(value string) {
	panic("not implemented")
}

func New() *HclThinParser {
	return &HclThinParser{}
}
