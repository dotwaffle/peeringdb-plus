package pdbcompat

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// This file ports the checks that upstream runs on a POST or PUT to the
// asset route before its permission check: the DRF parse of the body
// (request.py:326-380), the fields of AssetWriteSerializer and its
// validate (2.83.0 serializers.py:5090-5185). The mirror answers a
// request that passes every check with the 403 that upstream sends to
// a caller without write permission.

// assetMaxLogoBytes is ORG_LOGO_MAX_SIZE (settings/__init__.py:1728).
const assetMaxLogoBytes = 50 * 1024

// assetMaxLogoHeight is ORG_LOGO_MAX_VIEW_HEIGHT
// (settings/__init__.py:1732).
const assetMaxLogoHeight = 75

// pilMaxPixels is twice the Pillow MAX_IMAGE_PIXELS: Image.open raises
// DecompressionBombError for a larger image.
const pilMaxPixels = 2 * 89478485

// serveAssetWrite answers POST and PUT (create and update, rest.py:
// 1495-1558). The body is parsed first: create reads request.data
// before the serializer runs.
func (h *Handler) serveAssetWrite(w http.ResponseWriter, r *http.Request, p assetPath) {
	data, fail := parseAssetBody(r)
	if fail != nil {
		writeError(w, r, *fail)
		return
	}
	errs, fileType, fileData := assetWriteFieldErrors(p, data)
	if len(errs) > 0 {
		writeError(w, r, apiError{Status: http.StatusBadRequest, Fields: errs})
		return
	}
	if _, ok := h.assetLookup(w, r, p); !ok {
		return
	}
	if fe, ok := assetFileError(fileType, fileData); !ok {
		writeError(w, r, apiError{Status: http.StatusBadRequest, Fields: []fieldError{fe}})
		return
	}
	writeError(w, r, apiError{Status: http.StatusForbidden, Detail: errAssetNoWrite})
}

// maxFormFields is the Django DATA_UPLOAD_MAX_NUMBER_FIELDS default.
const maxFormFields = 1000

// parseAssetBody returns the dict {**request.data} of create and update
// (rest.py:1506-1511, :1540-1545), or the error response. The parser is
// the first of JSON, form and multipart whose media type matches the
// Content-Type (DRF negotiation.py:92-100). An empty body is an empty
// dict for every Content-Type (request.py:345-351).
//
// A form or multipart body is a QueryDict, and {**qd} copies its value
// lists: every value is a list of str, which no field accepts.
func parseAssetBody(r *http.Request) (pyValue, *apiError) {
	empty := pyValue{kind: pyDictValue}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return empty, &apiError{Status: http.StatusRequestEntityTooLarge, Detail: "Request body too large"}
		}
		return empty, &apiError{Status: http.StatusBadRequest, Detail: "failed to read request body"}
	}
	if len(body) == 0 {
		return empty, nil
	}
	contentType := r.Header.Get("Content-Type")
	fullType, params := djangoHeaderParams(contentType)
	codec := pyCodecOf(params["charset"])
	switch drfParserFor(fullType) {
	case "json":
		doc, err := pyDecode(body, codec)
		if err == nil {
			var v pyValue
			v, err = pyJSONLoads(doc)
			if err == nil {
				if v.kind != pyDictValue {
					// Upstream: {**<not a mapping>} raises TypeError, a
					// 500. The mirror sends the DRF Serializer error for
					// data that is not a dict (DRF serializers.py:497-503),
					// so a client cannot cause a 5xx.
					return empty, &apiError{Status: http.StatusBadRequest, Fields: []fieldError{{
						Field:   "non_field_errors",
						Message: "Invalid data. Expected a dictionary, but got " + pyTypeName(v.kind) + ".",
					}}}
				}
				return v, nil
			}
		}
		if errors.Is(err, errPyJSONDepth) {
			// Upstream: RecursionError, a 500. The mirror sends a 400
			// with the Python message, so a client cannot cause a 5xx.
			return empty, &apiError{Status: http.StatusBadRequest, Detail: "JSON parse error - maximum recursion depth exceeded while decoding a JSON document"}
		}
		return empty, &apiError{Status: http.StatusBadRequest, Detail: "JSON parse error - " + err.Error()}
	case "form":
		v, ok := parseDjangoQueryDict(body, codec)
		if !ok {
			return empty, &apiError{Status: http.StatusBadRequest, Detail: "Bad Request"}
		}
		return v, nil
	case "multipart":
		v, fail := parseDjangoMultipart(contentType, params, body, codec)
		if fail != "" {
			return empty, &apiError{Status: http.StatusBadRequest, Detail: "Multipart form parse error - " + fail}
		}
		return v, nil
	}
	return empty, &apiError{Status: http.StatusUnsupportedMediaType, Detail: `Unsupported media type "` + contentType + `" in request.`}
}

// pyTypeName is Python type(v).__name__ of a decoded JSON value.
func pyTypeName(k pyKind) string {
	switch k {
	case pyNoneKind:
		return "NoneType"
	case pyBoolKind:
		return "bool"
	case pyIntValue:
		return "int"
	case pyFloatValue:
		return "float"
	case pyStrValue:
		return "str"
	case pyListValue:
		return "list"
	case pyDictValue:
		return "dict"
	case pyReprValue:
		return "object"
	}
	return "object"
}

// djangoHeaderParams parses a Content-Type as Django
// parse_header_parameters does (django/utils/http.py): the lower-cased
// type, and the parameters with lower-cased names and unquoted values.
func djangoHeaderParams(line string) (string, map[string]string) {
	parts := djangoParseParam(";" + line)
	params := make(map[string]string)
	for _, p := range parts[1:] {
		name, value, ok := strings.Cut(p, "=")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		name = strings.TrimSuffix(name, "*")
		value = strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(value[1 : len(value)-1])
		}
		params[name] = value
	}
	return strings.ToLower(parts[0]), params
}

// djangoParseParam splits s at each ";" outside a quoted string, as
// Django _parseparam does, and strips the parts. s starts with ";".
func djangoParseParam(s string) []string {
	var parts []string
	for len(s) > 0 && s[0] == ';' {
		s = s[1:]
		end := strings.IndexByte(s, ';')
		for end > 0 && (strings.Count(s[:end], `"`)-strings.Count(s[:end], `\"`))%2 != 0 {
			next := strings.IndexByte(s[end+1:], ';')
			if next < 0 {
				end = -1
				break
			}
			end += 1 + next
		}
		if end < 0 {
			end = len(s)
		}
		parts = append(parts, strings.TrimSpace(s[:end]))
		s = s[end:]
	}
	return parts
}

// drfParserFor returns the DRF parser that the lower-cased media type
// selects: _MediaType.match with the parser type on the left, so "*"
// in the request type matches any parser type (mediatypes.py:52-63).
func drfParserFor(fullType string) string {
	main, sub, _ := strings.Cut(fullType, "/")
	for _, p := range []struct{ name, main, sub string }{
		{"json", "application", "json"},
		{"form", "application", "x-www-form-urlencoded"},
		{"multipart", "multipart", "form-data"},
	} {
		if (sub == "*" || sub == p.sub) && (main == "*" || main == p.main) {
			return p.name
		}
	}
	return ""
}

// parseDjangoQueryDict parses a form body as DRF FormParser does:
// QueryDict(body, encoding) (django/http/request.py QueryDict.__init__),
// with keep_blank_values and at most maxFormFields fields. A body that
// the encoding cannot decode is read as ISO-8859-1. ok is false when
// the body has too many fields (TooManyFieldsSent, a 400).
func parseDjangoQueryDict(body []byte, codec pyCodec) (pyValue, bool) {
	text, err := pyDecodeStrict(body, codec)
	if err != nil {
		text = pyLatin1Decode(body)
	}
	s := string(text)
	if strings.Count(s, "&")+1 > maxFormFields {
		return pyValue{}, false
	}
	v := pyValue{kind: pyDictValue}
	for field := range strings.SplitSeq(s, "&") {
		if field == "" {
			continue
		}
		name, value, _ := strings.Cut(field, "=")
		key := pyUnquote(strings.ReplaceAll(name, "+", " "), codec)
		val := pyValue{kind: pyStrValue, str: pyUnquote(strings.ReplaceAll(value, "+", " "), codec)}
		v.appendList(key, val)
	}
	return v, true
}

// appendList appends val to the list of the dict key, as
// QueryDict.appendlist does.
func (v *pyValue) appendList(key []rune, val pyValue) {
	for i, k := range v.keys {
		if slices.Equal(k, key) {
			v.vals[i].list = append(v.vals[i].list, val)
			return
		}
	}
	v.keys = append(v.keys, key)
	v.vals = append(v.vals, pyValue{kind: pyListValue, list: []pyValue{val}})
}

// pyUnquote is Python urllib.parse.unquote(s, encoding, "replace"):
// each run of ASCII text, with its %XX escapes as bytes, is decoded
// with the codec, and a bad sequence becomes U+FFFD. pyUnquotePlus is
// the UTF-8 form for a query string.
func pyUnquote(s string, codec pyCodec) []rune {
	var out []rune
	var buf []byte
	flush := func() {
		out = append(out, pyDecodeReplace(buf, codec)...)
		buf = buf[:0]
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r >= 0x80:
			flush()
			out = append(out, r)
		case r == '%' && i+2 < len(rs) && rs[i+1] < 0x80 && rs[i+2] < 0x80 && isHex(byte(rs[i+1])) && isHex(byte(rs[i+2])):
			buf = append(buf, unhex(byte(rs[i+1]))<<4|unhex(byte(rs[i+2])))
			i += 2
		default:
			buf = append(buf, byte(r))
		}
	}
	flush()
	return out
}

// djangoBoundary is the Django MultiPartParser boundary_re.
var djangoBoundary = regexp.MustCompile(`^[ -~]{0,200}[!-~]$`)

// parseDjangoMultipart parses a multipart body as DRF MultiPartParser
// does with the Django MultiPartParser (django/http/multipartparser.py):
// the checks of the Content-Type, then a list of values for each field
// name, text fields first, then files. A file shows as its repr. fail
// is the MultiPartParserError text of a bad Content-Type.
func parseDjangoMultipart(contentType string, params map[string]string, body []byte, codec pyCodec) (v pyValue, fail string) {
	if !strings.HasPrefix(contentType, "multipart/") {
		return pyValue{}, "Invalid Content-Type: " + contentType
	}
	for _, c := range contentType {
		if c >= 0x80 {
			return pyValue{}, "Invalid non-ASCII Content-Type in multipart: " + contentType
		}
	}
	boundary, ok := params["boundary"]
	if !ok || boundary == "" || !djangoBoundary.MatchString(boundary) {
		if !ok {
			boundary = "None"
		}
		return pyValue{}, "Invalid boundary in multipart: " + boundary
	}
	fields := pyValue{kind: pyDictValue}
	files := pyValue{kind: pyDictValue}
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := mr.NextRawPart()
		if err != nil {
			break
		}
		_, disp, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		name := disp["name"]
		if err != nil || name == "" {
			continue
		}
		data, _ := io.ReadAll(part)
		key := pyDecodeReplace([]byte(name), codec)
		fileName, isFile := disp["filename"]
		if !isFile {
			fields.appendList(key, pyValue{kind: pyStrValue, str: pyDecodeReplace(data, codec)})
			continue
		}
		fileName = sanitizeUploadName(fileName)
		if fileName == "" {
			continue
		}
		ct, _, _ := strings.Cut(part.Header.Get("Content-Type"), ";")
		repr := fmt.Sprintf("<InMemoryUploadedFile: %s (%s)>", fileName, strings.TrimSpace(ct))
		files.appendList(key, pyValue{kind: pyReprValue, str: []rune(repr)})
	}
	for i, k := range files.keys {
		for _, f := range files.vals[i].list {
			fields.appendList(k, f)
		}
	}
	return fields, ""
}

// sanitizeUploadName is Django sanitize_file_name without the HTML
// unescape: the name after the last "/" or "\", with the runes that
// Python does not print removed. "." and ".." are no name.
func sanitizeUploadName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if pyPrintable(r) {
			return r
		}
		return -1
	}, name)
	if name == "." || name == ".." {
		return ""
	}
	return name
}

// assetWriteFieldErrors checks the fields of AssetWriteSerializer in
// field order (serializers.py:5096-5122): the path values, then
// file_type (ChoiceField) and file_data (CharField). It returns the
// errors and the two body values.
func assetWriteFieldErrors(p assetPath, data pyValue) (errs []fieldError, fileType string, fileData []rune) {
	errs, _, _ = assetPathErrors(p)
	ft, present := data.get("file_type")
	switch {
	case !present:
		errs = append(errs, fieldError{Field: "file_type", Message: "This field is required."})
	case ft.kind == pyNoneKind:
		errs = append(errs, fieldError{Field: "file_type", Message: "This field may not be null."})
	default:
		s := ft.pyStr()
		if string(s) != "image/png" && string(s) != "image/jpeg" {
			errs = append(errs, fieldError{Field: "file_type", Message: invalidChoice(s)})
		} else {
			fileType = string(s)
		}
	}
	fd, present := data.get("file_data")
	switch {
	case !present:
		errs = append(errs, fieldError{Field: "file_data", Message: "This field is required."})
	case fd.kind == pyNoneKind:
		errs = append(errs, fieldError{Field: "file_data", Message: "This field may not be null."})
	case len(pyStripRunes(fd.pyStr())) == 0:
		errs = append(errs, fieldError{Field: "file_data", Message: "This field may not be blank."})
	case fd.kind != pyStrValue && fd.kind != pyIntValue && fd.kind != pyFloatValue:
		errs = append(errs, fieldError{Field: "file_data", Message: "Not a valid string."})
	default:
		fileData = pyStripRunes(fd.pyStr())
		if slices.Contains(fileData, 0) {
			errs = append(errs, fieldError{Field: "file_data", Message: "Null characters are not allowed."})
		}
		if i := slices.IndexFunc(fileData, isSurrogate); i >= 0 {
			errs = append(errs, fieldError{Field: "file_data", Message: fmt.Sprintf("Surrogate characters are not allowed: U+%X.", fileData[i])})
		}
	}
	return errs, fileType, fileData
}

// isSurrogate reports whether r is a UTF-16 surrogate code point.
func isSurrogate(r rune) bool {
	return 0xd800 <= r && r <= 0xdfff
}

// pyStripRunes is Python str.strip() with no argument.
func pyStripRunes(s []rune) []rune {
	start, end := 0, len(s)
	for start < end && pyIsSpace(s[start]) {
		start++
	}
	for end > start && pyIsSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

// assetFileError runs _validate_file_data (serializers.py:5124-5185) on
// the file_data text. ok is false with the error of the first check
// that fails.
func assetFileError(fileType string, fileData []rune) (fieldError, bool) {
	content, ok := pyB64Decode(fileData)
	if !ok {
		return fieldError{Field: "file_data", Message: "Invalid base64 encoded data"}, false
	}
	format, height := pilIdentify(content)
	if format == "" {
		return fieldError{Field: "file_data", Message: "Unsupported file type. Only PNG and JPEG are allowed"}, false
	}
	if want := "image/" + format; fileType != want {
		return fieldError{Field: "file_type", Message: "Declared file_type does not match actual file type. Expected: " + want}, false
	}
	if len(content) > assetMaxLogoBytes {
		return fieldError{Field: "file_data", Message: "File size too big, max. 50 kb"}, false
	}
	if height > assetMaxLogoHeight {
		return fieldError{Field: "file_data", Message: fmt.Sprintf("Image height too large. Maximum allowed: %d pixels", assetMaxLogoHeight)}, false
	}
	return fieldError{}, true
}

// pilIdentify returns the format ("png" or "jpeg") and the height of
// the image in b, as Pillow Image.open reads them, or "" when Pillow
// would not open b as one of them. For PNG, Pillow reads the chunks up
// to the first IDAT and checks their names and CRCs; for JPEG, it reads
// the markers up to the frame header.
func pilIdentify(b []byte) (string, int) {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		cfg, err := png.DecodeConfig(bytes.NewReader(b))
		if err != nil || !pngChunksOK(b[8:]) || cfg.Width*cfg.Height > pilMaxPixels {
			return "", 0
		}
		return "png", cfg.Height
	case bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}):
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
		if err != nil || cfg.Width*cfg.Height > pilMaxPixels {
			return "", 0
		}
		return "jpeg", cfg.Height
	}
	return "", 0
}

// pngChunkName is the Pillow is_cid pattern.
var pngChunkName = regexp.MustCompile(`^\w{4}$`)

// pngChunksOK reports whether the chunks of a PNG body up to the first
// IDAT are complete, have Pillow chunk names and have good CRCs.
func pngChunksOK(b []byte) bool {
	for {
		if len(b) < 8 {
			return false
		}
		n := int(uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]))
		name := b[4:8]
		if !pngChunkName.Match(name) {
			return false
		}
		if string(name) == "IDAT" {
			return true
		}
		if n < 0 || len(b) < 12+n {
			return false
		}
		want := uint32(b[8+n])<<24 | uint32(b[9+n])<<16 | uint32(b[10+n])<<8 | uint32(b[11+n])
		if crc32.ChecksumIEEE(b[4:8+n]) != want {
			return false
		}
		b = b[12+n:]
	}
}
