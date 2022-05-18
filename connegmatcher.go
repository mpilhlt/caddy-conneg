// Copyright 2022 Andreas Wagner
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package connegmatcher

import (
	"errors"
	"net/http"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/elnormous/contenttype"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// Parameters is a map to represent charset or encoding parameters.
type Parameters = map[string]string

// CharsetOrEncoding is s structure to represent charset or encoding with their parameters.
type CharsetOrEncoding struct {
	Value      string
	Parameters Parameters
}

// MatchConneg matches requests by comparing results of a
// content negotiation process to a (list of) value(s).
//
// Lists of media types, languages, charsets, and encodings to match
// the request against can be given - and at least one of them MUST
// be specified.
//
// OPTIONAL parameters are strings for identifying URL query string
// parameter keys that allow requests to override/skip the connection
// negotiation process and force a media type, a language, a charset
// or an encoding.
//
// Some shorthand values for query string parameters translating to
// full media types (languages, encodings, etc.) are hardcoded in a
// variable called `aliases`: They presently cover `htm` and `html` for
// `text/html`, `rdf` for `application/rdf+xml`, `tei` and `xml` for
// `application/tei+xml`, and `pdf` for `application/pdf`. For instance,
// if `force_type_query_string` is set to `format`, a request uri
// ending in `foo.com?format=tei` will result in content type
// `application/tei+xml` and then succeed or not based on whether that
// content type is listed in `match_types`.
//
// COMPATIBILITY NOTE: This module is still experimental and is not
// subject to Caddy's compatibility guarantee.
type MatchConneg struct {
	// List of content/mime types to match against (W3C RFC 2616, section 14.1). Default: Empty list
	MatchTypes               []string `json:"match_types,omitempty"`
	// List of language codes to match against (W3C RFC 2616, section 14.4). Default: Empty list
	MatchLanguages           []string `json:"match_languages,omitempty"`
	// List of character sets to match against (W3C RFC 2616, section 14.2). Default: Empty list
	MatchCharsets            []string `json:"match_charsets,omitempty"`
	// List of encodings to match against (W3C RFC 2616, section 14.3). Default: Empty list
	MatchEncodings           []string `json:"match_encodings,omitempty"`
	// Query string parameter key to override content negotiation. Default: ""
	ForceTypeQueryString     string   `json:"force_type_query_string,omitempty"`
	// Query string parameter key to override language negotiation. Default: ""
	ForceLanguageQueryString string   `json:"force_language_query_string,omitempty"`
	// Query string parameter key to override charset negotiation. Default: ""
	ForceCharsetQueryString  string   `json:"force_charset_query_string,omitempty"`
	// Query string parameter key to override encoding negotiation. Default: ""
	ForceEncodingQueryString string   `json:"force_encoding_query_string,omitempty"`
	// Variable name (will be prefixed with `conneg_`) to hold result of content negotiation. Default: ""
	VarType                  string   `json:"var_type, omitempty`
	// Variable name (will be prefixed with `conneg_`) to hold result of language negotiation. Default: ""
	VarLanguage              string   `json:"var_language, omitempty`
	// Variable name (will be prefixed with `conneg_`) to hold result of charset negotiation. Default: ""
	VarCharset               string   `json:"var_charset, omitempty`
	// Variable name (will be prefixed with `conneg_`) to hold result of encoding negotiation. Default: ""
	VarEncoding              string   `json:"var_encoding, omitempty`

	// the following fields are populated internally/computationally
	MatchTTypes     []contenttype.MediaType
	MatchTLanguages []language.Tag
	MatchTCharsets  []CharsetOrEncoding
	MatchTEncodings []CharsetOrEncoding
	LanguageMatcher language.Matcher
	logger          *zap.Logger
}

// If a type/language/etc is forced via parameter, these are values that the parameter can take
var aliases = map[string]interface{}{
	"text/html":           []string{"html", "htm"},
	"application/rdf+xml": []string{"rdf"},
	"application/tei+xml": []string{"tei", "xml"},
	"application/pdf":     []string{"pdf"},
}

func init() {
	caddy.RegisterModule(MatchConneg{})
}

// CaddyModule returns the Caddy module information.
func (MatchConneg) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.matchers.conneg",
		New: func() caddy.Module { return new(MatchConneg) },
	}
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (m *MatchConneg) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		for nesting := d.Nesting(); d.NextBlock(nesting); {
			switch d.Val() {
			case "match_types":
				m.MatchTypes = append(m.MatchTypes, d.RemainingArgs()...)
			case "match_languages":
				m.MatchLanguages = append(m.MatchLanguages, d.RemainingArgs()...)
			case "match_charsets":
				m.MatchCharsets = append(m.MatchCharsets, d.RemainingArgs()...)
			case "match_encodings":
				m.MatchEncodings = append(m.MatchEncodings, d.RemainingArgs()...)
			case "force_type_query_string":
				d.Next()
				m.ForceTypeQueryString = d.Val()
			case "force_language_query_string":
				d.Next()
				m.ForceLanguageQueryString = d.Val()
			case "force_charset_query_string":
				d.Next()
				m.ForceCharsetQueryString = d.Val()
			case "force_encoding_query_string":
				d.Next()
				m.ForceEncodingQueryString = d.Val()
			case "var_type":
				d.Next()
				m.VarType = d.Val()
			case "var_language":
				d.Next()
				m.VarLanguage = d.Val()
			case "var_charset":
				d.Next()
				m.VarCharset = d.Val()
			case "var_encoding":
				d.Next()
				m.VarEncoding = d.Val()
			}
		}
	}
	return nil
}

// Provision sets up the module.
func (m *MatchConneg) Provision(ctx caddy.Context) error {
	// m.logger = ctx.Logger(m) // m.logger is a *zap.Logger
	// sugar := m.logger.Sugar()
	// defer m.logger.Sync() // flushes buffer, if any

	for _, t := range m.MatchTypes {
		m.MatchTTypes = append(m.MatchTTypes, contenttype.NewMediaType(t))
	}

	m.MatchTLanguages = append(m.MatchTLanguages, language.Make("und"))
	for _, l := range m.MatchLanguages {
		m.MatchTLanguages = append(m.MatchTLanguages, language.Make(l))
	}
	m.LanguageMatcher = language.NewMatcher(m.MatchTLanguages)

	for _, c := range m.MatchCharsets {
		m.MatchTCharsets = append(m.MatchTCharsets, CharsetOrEncoding{Value: c})
	}

	for _, e := range m.MatchEncodings {
		m.MatchTEncodings = append(m.MatchTEncodings, CharsetOrEncoding{Value: e})
	}

	// sugar.Infof("Conneg config: %+v", m)
	return nil
}

// Validate validates that the module has a usable config.
func (m MatchConneg) Validate() error {
	if len(m.MatchTypes)+len(m.MatchLanguages)+len(m.MatchCharsets)+len(m.MatchEncodings) == 0 {
		return errors.New("One of match_types, match_languages, match_charsets, match_encodings MUST be set.")
	}
	if len(m.MatchTypes) == 0 && len(m.VarType) > 0 {
		return errors.New("You cannot specify a variable to store content negotiation results (for content types) if you don't also specify what types are offered. (Use '*/*' to work around this constraint.)")
	}
	if len(m.MatchLanguages) == 0 && len(m.VarLanguage) > 0 {
		return errors.New("You cannot specify a variable to store content negotiation results (for languages) if you don't also specify what languages are offered. (Use '*' to work around this constraint.)")
	}
	if len(m.MatchCharsets) == 0 && len(m.VarCharset) > 0 {
		return errors.New("You cannot specify a variable to store content negotiation results (for charsets) if you don't also specify what charsets are offered. (Use '*' to work around this constraint.)")
	}
	if len(m.MatchEncodings) == 0 && len(m.VarEncoding) > 0 {
		return errors.New("You cannot specify a variable to store content negotiation results (for encodings) if you don't also specify what encodings are offered. (Use '*' to work around this constraint.)")
	}
	return nil
}

// Match returns true if the request matches all requirements.
func (m MatchConneg) Match(r *http.Request) bool {

	typeMatch, _type := false, ""
	if len(m.MatchTypes) == 0 {
		typeMatch = true
	} else {
		typeMatch, _type = m.matchType(r, m.MatchTypes, m.MatchTTypes, m.ForceTypeQueryString, "Accept")
		if typeMatch && len(m.VarType) > 0 {
			caddyhttp.SetVar(r.Context(), "conneg_"+m.VarType, _type)
		}
	}

	languageMatch, language := false, ""
	if len(m.MatchLanguages) == 0 {
		languageMatch = true
	} else {
		languageMatch, language = m.matchLanguage(r, m.MatchLanguages, m.ForceLanguageQueryString, "Accept-Language")
		if languageMatch && len(m.VarLanguage) > 0 {
			caddyhttp.SetVar(r.Context(), "conneg_"+m.VarLanguage, language)
		}
	}

	charsetMatch, charset := false, ""
	if len(m.MatchCharsets) == 0 {
		charsetMatch = true
	} else {
		charsetMatch, charset = m.matchCharsetOrEncoding(r, m.MatchCharsets, m.MatchTCharsets, m.ForceCharsetQueryString, "Accept-Charset")
		if charsetMatch && len(m.VarCharset) > 0 {
			caddyhttp.SetVar(r.Context(), "conneg_"+m.VarCharset, charset)
		}
	}

	encodingMatch, encoding := false, ""
	if len(m.MatchEncodings) == 0 {
		encodingMatch = true
	} else {
		encodingMatch, encoding = m.matchCharsetOrEncoding(r, m.MatchEncodings, m.MatchTEncodings, m.ForceEncodingQueryString, "Accept-Encoding")
		if encodingMatch && len(m.VarEncoding) > 0 {
			caddyhttp.SetVar(r.Context(), "conneg_"+m.VarEncoding, encoding)
		}
	}

	return (typeMatch && languageMatch && charsetMatch && encodingMatch)
}

func (m MatchConneg) matchType(r *http.Request, offers []string, offerTypes []contenttype.MediaType, forceString string, headerName string) (bool, string) {
	match, result := false, ""
	if forceString != "" {
		if err := r.ParseForm(); err != nil {
			sugar := m.logger.Sugar()
			sugar.Infof("Problem parsing URL: %+v", err)
			// return errors.New("One of match_types, match_languages, match_charsets, match_encodings MUST be set.")
		} else {
			if len(r.Form[forceString]) > 0 {
				for _, t := range offers {
					if t == r.Form[forceString][0] {
						match, result = true, t
					} else {
						values, containsKey := aliases[t]
						if containsKey {
							if slices.Contains(values.([]string), r.Form[forceString][0]) {
								match, result = true, t
							}
						}
					}
				}
				if !match {
					return false, ""
				}
			}
		}
	}
	if !match {
		var headerValues []string
		headerValues = append(headerValues, r.Header.Values(headerName)...)
		for _, a := range headerValues {
			var mediatype, _, _ = contenttype.GetAcceptableMediaTypeFromHeader(a, offerTypes)
			if mediatype.Type != "" {
				match, result = true, mediatype.String()
			}
		}
	}
	return match, result
}

func (m MatchConneg) matchLanguage(r *http.Request, offers []string, forceString string, headerName string) (bool, string) {

	match, result := false, ""
	if forceString != "" {
		if err := r.ParseForm(); err != nil {
			sugar := m.logger.Sugar()
			sugar.Infof("Problem parsing URL: %+v", err)
			// return errors.New("One of match_types, match_languages, match_charsets, match_encodings MUST be set.")
		} else {
			if len(r.Form[forceString]) > 0 {
				for _, t := range offers {
					if t == r.Form[forceString][0] {
						match, result = true, t
					} else {
						values, containsKey := aliases[t]
						if containsKey {
							if slices.Contains(values.([]string), r.Form[forceString][0]) {
								match, result = true, t
							}
						}
					}
				}
				if !match {
					return false, ""
				}
			}
		}
	}
	if !match {
		var headerValues []string
		headerValues = append(headerValues, r.Header.Values(headerName)...)
		tag, _ := language.MatchStrings(m.LanguageMatcher, strings.Join(headerValues, ", "))
		match = !tag.IsRoot()
		if match {
			result = display.English.Tags().Name(tag) + "/" + display.Self.Name(tag)
		} else {
			result = ""
		}
	}
	return match, result
}

func (m MatchConneg) matchCharsetOrEncoding(r *http.Request, offers []string, offerCharsetOrEncodings []CharsetOrEncoding, forceString string, headerName string) (bool, string) {
	match, result := false, ""
	if forceString != "" {
		if err := r.ParseForm(); err != nil {
			sugar := m.logger.Sugar()
			sugar.Infof("Problem parsing URL: %+v", err)
			// return errors.New("One of match_types, match_languages, match_charsets, match_encodings MUST be set.")
		} else {
			if len(r.Form[forceString]) > 0 {
				for _, t := range offers {
					if t == r.Form[forceString][0] {
						match, result = true, t
					} else {
						values, containsKey := aliases[t]
						if containsKey {
							if slices.Contains(values.([]string), r.Form[forceString][0]) {
								match, result = true, t
							}
						}
					}
				}
				if !match {
					return false, ""
				}
			}
		}
	}
	if !match {
		var headerValues []string
		headerValues = append(headerValues, r.Header.Values(headerName)...)
		for _, a := range headerValues {
			var other, _, _ = getAcceptableCharsetOrEncodingFromHeader(a, offerCharsetOrEncodings)
			if other.Value != "" {
				match, result = true, other.Value
			}
		}
	}
	return match, result
}

// Interface guards
var (
	_ caddyhttp.RequestMatcher = (*MatchConneg)(nil)
	_ caddyfile.Unmarshaler    = (*MatchConneg)(nil)
	_ caddy.Provisioner        = (*MatchConneg)(nil)
	_ caddy.Validator          = (*MatchConneg)(nil)
)

/*
Functions signatures from other packages:

Last Change	Function signature																								Package (RFC)						URL
--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
20210922	func ParseMediaType(v string) (mediatype string, params map[string]string, err error) 							mime (1521, 2183) 					<https://pkg.go.dev/mime#ParseMediaType>
20210809	func ParseAcceptLanguage(s string) (tag []Tag, q []float32, err error) 											x/text/language (2616) 				<https://pkg.go.dev/golang.org/x/text@v0.3.7/language#ParseAcceptLanguage>
20220510	func GetAcceptableMediaTypeFromHeader(hValue string, offerTypes []MediaType) (MediaType, Parameters, error) 	elnormous/contenttype (7231) 		<https://github.com/elnormous/contenttype>
20190713	func ParseAccept(header string) []Accept																		markusthoemmes/goautoneg 			<https://github.com/markusthoemmes/goautoneg/blob/master/accept.go>
20181109	func NegotiateAcceptHeader(header http.Header, key string, offers []string) string 								lokhman/gowl 						<https://github.com/lokhman/gowl/blob/master/httputil/negotiate.go>
20140225	func NegotiateContentEncoding(r *http.Request, offers []string) string											gddo/httputil						<https://github.com/golang/gddo/blob/master/httputil/negotiate.go>
20130320	func Parse(header string) AcceptSlice																			timewasted/go-accept-headers (2616)	<https://github.com/timewasted/go-accept-headers>
*/

func isChar(c byte) bool {
	// token    = 1*<any CHAR except CTLs or separators>
	// isChar	= 0 <= c && c <= 127
	// isCTL    = c <= 31 || c == 127
	// isSep    = strings.IndexRune(" \t\"(),/:;<=>?@[]\\{}", rune(c)) >= 0
	return 32 <= c && c <= 126 &&
		strings.IndexRune(" \t\"(),/:;<=>?@[]\\{}", rune(c)) == -1
}

func isWhitespaceChar(c byte) bool {
	// RFC 7230, 3.2.3. Whitespace
	return c == 0x09 || c == 0x20 // HTAB or SP
}

func isDigitChar(c byte) bool {
	// RFC 5234, Appendix B.1. Core Rules
	return c >= 0x30 && c <= 0x39
}

func skipSpace(s string) (rest string) {
	for i := 0; i < len(s); i++ {
		if !isWhitespaceChar(s[i]) {
			return s[i:]
		}
	}
	return ""
}

func consumeToken(s string) (token, remaining string, consumed bool) {
	// RFC 7230, 3.2.6. Field Value Components
	for i := 0; i < len(s); i++ {
		if !isChar(s[i]) {
			return strings.ToLower(s[:i]), s[i:], i > 0
		}
	}

	return strings.ToLower(s), "", len(s) > 0
}

func consumeParameter(s string) (string, string, string, bool) {
	s = skipSpace(s)

	var consumed bool
	var key string
	if key, s, consumed = consumeToken(s); !consumed {
		return "", "", s, false
	}

	if len(s) == 0 || s[0] != '=' {
		return "", "", s, false
	}

	s = s[1:] // skip the equal sign

	var value string
	if value, s, consumed = consumeToken(s); !consumed {
		return "", "", s, false
	}

	s = skipSpace(s)

	return key, value, s, true
}

func getWeight(s string) (int, bool) {
	// RFC 7231, 5.3.1. Quality Values
	result := 0
	multiplier := 1000

	// the string must not have more than three digits after the decimal point
	if len(s) > 5 {
		return 0, false
	}

	for i := 0; i < len(s); i++ {
		if i == 0 {
			// the first character must be 0 or 1
			if s[i] != '0' && s[i] != '1' {
				return 0, false
			}

			result = int(s[i]-'0') * multiplier
			multiplier /= 10
		} else if i == 1 {
			// the second character must be a dot
			if s[i] != '.' {
				return 0, false
			}
		} else {
			// the remaining characters must be digits and the value can not be greater than 1.000
			if (s[0] == '1' && s[i] != '0') ||
				!isDigitChar(s[i]) {
				return 0, false
			}

			result += int(s[i]-'0') * multiplier
			multiplier /= 10
		}
	}

	return result, true
}

func compareCharsetOrEncodings(checkCharsetOrEncoding, other CharsetOrEncoding) bool {
	// RFC 7231, 5.3.2. Accept
	if other.Value == "*" || checkCharsetOrEncoding.Value == other.Value {

		for checkKey, checkValue := range checkCharsetOrEncoding.Parameters {
			if value, found := other.Parameters[checkKey]; !found || value != checkValue {
				return false
			}
		}

		return true
	}

	return false
}

func getPrecedence(checkCharsetOrEncoding, other CharsetOrEncoding) bool {
	// RFC 7231, 5.3.2. Accept
	if len(other.Value) == 0 { // not set
		return true
	}

	if (other.Value == "*" && checkCharsetOrEncoding.Value != "*") ||
		(len(other.Parameters) < len(checkCharsetOrEncoding.Parameters)) {
		return true
	}

	return false
}

// getAcceptableCharsetOrEncodingFromHeader chooses a charset or encoding from available lists according to the specified Accept header value.
// Returns the most charset/encoding or an error if none can be selected.
// This is copied from <> and modified only slightly
func getAcceptableCharsetOrEncodingFromHeader(headerValue string, availableCharsetOrEncodings []CharsetOrEncoding) (CharsetOrEncoding, Parameters, error) {
	s := headerValue

	weights := make([]struct {
		other               CharsetOrEncoding
		extensionParameters Parameters
		weight              int
		order               int
	}, len(availableCharsetOrEncodings))

	for otherCount := 0; len(s) > 0; otherCount++ {
		if otherCount > 0 {
			// every entry after the first one must start with a comma
			if s[0] != ',' {
				break
			}
			s = s[1:] // skip the comma
		}

		acceptableCharsetOrEncoding := CharsetOrEncoding{
			Parameters: Parameters{},
		}
		var consumed bool
		if acceptableCharsetOrEncoding.Value, s, consumed = consumeToken(s); !consumed {
			return CharsetOrEncoding{}, Parameters{}, errors.New("invalid value in Accept-* string")
		}

		weight := 1000 // 1.000

		// parameters
		for len(s) > 0 && s[0] == ';' {
			s = s[1:] // skip the semicolon

			var key, value string
			if key, value, s, consumed = consumeParameter(s); !consumed {
				return CharsetOrEncoding{}, Parameters{}, errors.New("invalid parameter in Accept-* string")
			}

			if key == "q" {
				if weight, consumed = getWeight(value); !consumed {
					return CharsetOrEncoding{}, Parameters{}, errors.New("invalid weight in Accept-* string")
				}
				break // "q" parameter separates media type parameters from Accept extension parameters
			}

			acceptableCharsetOrEncoding.Parameters[key] = value
		}

		extensionParameters := Parameters{}
		for len(s) > 0 && s[0] == ';' {
			s = s[1:] // skip the semicolon

			var key, value, remaining string
			if key, value, remaining, consumed = consumeParameter(s); !consumed {
				return CharsetOrEncoding{}, Parameters{}, errors.New("invalid parameter in Accept-* string")
			}

			s = remaining

			extensionParameters[key] = value
		}

		for i, availableCharsetOrEncoding := range availableCharsetOrEncodings {
			if compareCharsetOrEncodings(acceptableCharsetOrEncoding, availableCharsetOrEncoding) &&
				getPrecedence(acceptableCharsetOrEncoding, weights[i].other) {
				weights[i].other = acceptableCharsetOrEncoding
				weights[i].extensionParameters = extensionParameters
				weights[i].weight = weight
				weights[i].order = otherCount
			}
		}

		s = skipSpace(s)
	}

	// there must not be anything left after parsing the header
	if len(s) > 0 {
		return CharsetOrEncoding{}, Parameters{}, errors.New("invalid range in Accept-* string")
	}

	resultIndex := -1
	for i, weight := range weights {
		if resultIndex != -1 {
			if weight.weight > weights[resultIndex].weight ||
				(weight.weight == weights[resultIndex].weight && weight.order < weights[resultIndex].order) {
				resultIndex = i
			}
		} else if weight.weight > 0 {
			resultIndex = i
		}
	}

	if resultIndex == -1 {
		return CharsetOrEncoding{}, Parameters{}, errors.New("no acceptable value found")
	}

	return availableCharsetOrEncodings[resultIndex], weights[resultIndex].extensionParameters, nil
}
