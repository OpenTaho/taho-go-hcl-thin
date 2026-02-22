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
	HclAlphaNumeric = iota
	HclEquals
	HclNewLine
	HclOther
	HclPound
	HclSpace
	HclUnknown
)

type HclThinElement struct {
	hcl.HclElement

	pair    hcl.HclElement
	value   string
	comment bool
}

type HclThinDir struct {
	name   string
	parser hcl.HclParser
}

type HclThinFile struct {
	name string
}

type HclThinParser struct{}

func New() *HclThinParser {
	return &HclThinParser{}
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

func (h *HclThinDir) Name() string {
	return h.name
}

func (*HclThinDir) SetName(value string) {
	panic("not implemented")
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

func (h *HclThinFile) Elements() []hcl.HclElement {
	elements := []hcl.HclElement{}

	f, err := os.Open(h.name)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	c := HclUnknown
	cc := HclAlphaNumeric
	ignore := false
	isComment := false
	pairing := false
	pairingSoon := false
	pairingNext := false
	pairingPending1 := false
	pairingPending2 := false
	br := bufio.NewReader(f)
	var sb strings.Builder

	for {
		r, _, err := br.ReadRune()
		willBreak := false
		if err != nil {
			r = ' '
			willBreak = true
		}

		if isAlphaNumeric(r) {
			c = HclAlphaNumeric
		} else if r == '\n' {
			c = HclNewLine
			cc = HclUnknown
		} else if isSpace(r) {
			c = HclSpace
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
			isComment = true
			readComment(br, &sb)
			ignore = true
		} else {
			c = HclOther
			cc = HclUnknown
		}

		if cc != c {
			v := sb.String()
			if v != "" {
				if pairing {
					pairing = false
					v = elements[len(elements)-1].Value()
					elements = elements[:len(elements)-4]
					last := len(elements) - 1
					elements[last] = &HclThinElement{
						value: v,
						pair:  elements[last],
					}
				} else {
					lastWasComment := len(elements) > 0 && elements[len(elements)-1].Comment()
					if isComment && lastWasComment {
						v = elements[len(elements)-1].Value() + v
						elements[len(elements)-1].SetValue(v)
					} else {
						elements = append(elements, &HclThinElement{
							value:   v,
							comment: isComment,
						})
						if pairingNext {
							pairingNext = false
							pairing = true
						}
						if pairingPending2 {
							pairingPending2 = false
							pairingNext = true
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

		isComment = false

		if willBreak {
			break
		} else if !ignore {
			sb.WriteRune(r)
		} else {
			ignore = false
		}
	}

	return elements
}

func readComment(br *bufio.Reader, sb *strings.Builder) {
	s, err := br.ReadString('\n')
	if err != nil {
		panic(err)
	}
	sb.WriteString(s)
}

func (h *HclThinElement) SetValue(value string) {
	h.value = value
}

func (h *HclThinElement) Comment() bool {
	return h.comment
}

func (h *HclThinElement) Value() string {
	return h.value
}

func (h *HclThinElement) SetPair(pair hcl.HclElement) {
	h.pair = pair
}

func (h *HclThinElement) Pair() hcl.HclElement {
	return h.pair
}

func (h *HclThinFile) Name() string {
	return h.name
}

func (*HclThinFile) SetName(value string) {
	panic("not implemented")
}

func isAlphaNumeric(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

func isSpace(r rune) bool {
	return unicode.IsSpace(r)
}
