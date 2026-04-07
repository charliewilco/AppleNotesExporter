package convert

import (
	"fmt"
	"mime"
	"net/url"
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type AssetSourceType string

const (
	AssetSourceDataURL AssetSourceType = "data"
	AssetSourceRemote  AssetSourceType = "remote"
)

type Options struct {
	AssetDir    string
	AssetPrefix string
	NoImages    bool
}

type Result struct {
	Markdown string
	Assets   []Asset
}

type Asset struct {
	RelativePath string
	SourceType   AssetSourceType
	Source       string
}

type renderer struct {
	options    Options
	assets     []Asset
	imageIndex int
}

var whitespacePattern = regexp.MustCompile(`\s+`)
var markdownEscapeReplacer = strings.NewReplacer(
	`\\`, `\\\\`,
	"`", "\\`",
	"[", "\\[",
	"]", "\\]",
)

func HTMLToMarkdown(input string, options Options) (Result, error) {
	document, err := html.Parse(strings.NewReader("<!doctype html><html><body>" + input + "</body></html>"))
	if err != nil {
		return Result{}, fmt.Errorf("parse HTML: %w", err)
	}

	body := findElement(document, "body")
	if body == nil {
		return Result{}, fmt.Errorf("parse HTML: missing body node")
	}

	renderer := renderer{
		options: options,
	}

	sections := renderer.renderContainer(body)
	markdown := normalizeMarkdown(strings.Join(sections, "\n\n"))

	return Result{
		Markdown: markdown,
		Assets:   renderer.assets,
	}, nil
}

func (renderer *renderer) renderNode(node *html.Node) []string {
	switch node.Type {
	case html.TextNode:
		text := normalizeParagraph(renderer.renderInline(node))
		if text == "" {
			return nil
		}

		return []string{text}
	case html.ElementNode:
	default:
		return nil
	}

	switch node.Data {
	case "h1", "h2", "h3":
		title := normalizeParagraph(renderer.renderInlineChildren(node))
		if title == "" {
			return nil
		}

		level := 1
		if len(node.Data) == 2 {
			level = int(node.Data[1] - '0')
		}

		return []string{strings.Repeat("#", level) + " " + title}
	case "ul", "ol":
		list := renderer.renderList(node, 0)
		if list == "" {
			return nil
		}

		return []string{list}
	case "table":
		table := renderer.renderTable(node)
		if table == "" {
			return nil
		}

		return []string{table}
	case "blockquote":
		quoted := renderer.renderBlockquote(node)
		if quoted == "" {
			return nil
		}

		return []string{quoted}
	case "pre":
		code := normalizeCodeBlock(renderer.extractTextWithBreaks(node))
		if code == "" {
			return nil
		}

		return []string{"```\n" + code + "\n```"}
	case "div", "p":
		if hasOnlyLineBreaks(node) {
			return []string{""}
		}

		if isMonospaceBlock(node) {
			code := normalizeCodeBlock(renderer.extractTextWithBreaks(node))
			if code != "" {
				return []string{"```\n" + code + "\n```"}
			}
		}

		return renderer.renderContainer(node)
	case "body", "section", "article", "object":
		return renderer.renderContainer(node)
	default:
		return renderer.renderContainer(node)
	}
}

func (renderer *renderer) renderContainer(node *html.Node) []string {
	var sections []string
	var inlineParts []string

	flushInline := func() {
		if len(inlineParts) == 0 {
			return
		}

		text := normalizeParagraph(strings.Join(inlineParts, ""))
		inlineParts = inlineParts[:0]
		if text != "" {
			sections = append(sections, text)
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if isBlockNode(child) {
			flushInline()
			sections = append(sections, renderer.renderNode(child)...)
			continue
		}

		inlineParts = append(inlineParts, renderer.renderInline(child))
	}

	flushInline()

	if len(sections) == 0 && hasOnlyLineBreaks(node) {
		return []string{""}
	}

	return sections
}

func (renderer *renderer) renderInline(node *html.Node) string {
	switch node.Type {
	case html.TextNode:
		return escapeMarkdownText(compactWhitespace(node.Data))
	case html.ElementNode:
	default:
		return ""
	}

	switch node.Data {
	case "strong", "b":
		content := normalizeParagraph(renderer.renderInlineChildren(node))
		if content == "" {
			return ""
		}

		return "**" + content + "**"
	case "em", "i":
		content := normalizeParagraph(renderer.renderInlineChildren(node))
		if content == "" {
			return ""
		}

		return "*" + content + "*"
	case "s", "strike", "del":
		content := normalizeParagraph(renderer.renderInlineChildren(node))
		if content == "" {
			return ""
		}

		return "~~" + content + "~~"
	case "a":
		href := strings.TrimSpace(attribute(node, "href"))
		label := normalizeParagraph(renderer.renderInlineChildren(node))
		if label == "" {
			label = href
		}
		if href == "" {
			return label
		}

		return fmt.Sprintf("[%s](%s)", label, href)
	case "br":
		return "\n"
	case "img":
		return renderer.renderImage(node)
	case "input":
		if strings.EqualFold(attribute(node, "type"), "checkbox") {
			if hasTruthyAttribute(node, "checked") {
				return "[x] "
			}

			return "[ ] "
		}

		return ""
	case "code":
		content := normalizeParagraph(renderer.renderInlineChildren(node))
		if content == "" {
			return ""
		}

		return "`" + strings.ReplaceAll(content, "`", "\\`") + "`"
	case "span":
		content := renderer.renderInlineChildren(node)
		if isMonospaceNode(node) {
			code := normalizeParagraph(content)
			if code == "" {
				return ""
			}

			return "`" + strings.ReplaceAll(code, "`", "\\`") + "`"
		}

		return content
	default:
		return renderer.renderInlineChildren(node)
	}
}

func (renderer *renderer) renderInlineChildren(node *html.Node) string {
	var parts strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		parts.WriteString(renderer.renderInline(child))
	}

	return parts.String()
}

func (renderer *renderer) renderList(node *html.Node, depth int) string {
	ordered := node.Data == "ol"
	index := 1
	var lines []string

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || child.Data != "li" {
			continue
		}

		lines = append(lines, renderer.renderListItem(child, depth, ordered, index)...)
		index++
	}

	return strings.Join(lines, "\n")
}

func (renderer *renderer) renderListItem(node *html.Node, depth int, ordered bool, index int) []string {
	indent := strings.Repeat("  ", depth)
	marker := "- "
	if ordered {
		marker = fmt.Sprintf("%d. ", index)
	}

	var inlineParts []string
	var nestedBlocks []string

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && (child.Data == "ul" || child.Data == "ol") {
			nestedBlocks = append(nestedBlocks, renderer.renderList(child, depth+1))
			continue
		}

		if isBlockNode(child) && child.Data != "div" && child.Data != "p" {
			inlineParts = append(inlineParts, strings.Join(renderer.renderNode(child), "\n"))
			continue
		}

		if child.Type == html.ElementNode && isMonospaceBlock(child) {
			inlineParts = append(inlineParts, renderer.extractTextWithBreaks(child))
			continue
		}

		inlineParts = append(inlineParts, renderer.renderInline(child))
	}

	text := normalizeParagraph(strings.Join(inlineParts, ""))
	lines := formatListLines(indent, marker, text)
	if len(lines) == 0 {
		lines = []string{indent + strings.TrimRight(marker, " ")}
	}

	for _, nested := range nestedBlocks {
		if strings.TrimSpace(nested) == "" {
			continue
		}
		lines = append(lines, nested)
	}

	return lines
}

func (renderer *renderer) renderTable(node *html.Node) string {
	var rows [][]string
	visit(node, func(candidate *html.Node) {
		if candidate.Type != html.ElementNode || candidate.Data != "tr" {
			return
		}

		var cells []string
		for child := candidate.FirstChild; child != nil; child = child.NextSibling {
			if child.Type != html.ElementNode || (child.Data != "td" && child.Data != "th") {
				continue
			}

			cellText := normalizeParagraph(renderer.renderInlineChildren(child))
			cellText = strings.ReplaceAll(cellText, "\n", "<br>")
			cells = append(cells, escapeTableCell(cellText))
		}

		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	})

	if len(rows) == 0 {
		return ""
	}

	columnCount := 0
	for _, row := range rows {
		if len(row) > columnCount {
			columnCount = len(row)
		}
	}

	for index := range rows {
		for len(rows[index]) < columnCount {
			rows[index] = append(rows[index], "")
		}
	}

	header := "| " + strings.Join(rows[0], " | ") + " |"
	separator := "|" + strings.Repeat(" --- |", columnCount)

	lines := []string{header, separator}
	for _, row := range rows[1:] {
		lines = append(lines, "| "+strings.Join(row, " | ")+" |")
	}

	return strings.Join(lines, "\n")
}

func (renderer *renderer) renderBlockquote(node *html.Node) string {
	sections := renderer.renderContainer(node)
	if len(sections) == 0 {
		return ""
	}

	return prefixLines(strings.Join(sections, "\n\n"), "> ")
}

func (renderer *renderer) renderImage(node *html.Node) string {
	altText := normalizeParagraph(attribute(node, "alt"))
	if altText == "" {
		altText = "image"
	}

	source := strings.TrimSpace(attribute(node, "src"))
	if source == "" {
		return fmt.Sprintf("[Image: %s]", altText)
	}

	if renderer.options.NoImages {
		return fmt.Sprintf("[Image omitted: %s]", altText)
	}

	relativePath, ok := renderer.registerAsset(source)
	if !ok {
		return fmt.Sprintf("[Image: %s]", altText)
	}

	return fmt.Sprintf("![%s](%s)", altText, relativePath)
}

func (renderer *renderer) registerAsset(source string) (string, bool) {
	source = strings.TrimSpace(source)
	sourceType := AssetSourceRemote

	switch {
	case strings.HasPrefix(strings.ToLower(source), "data:"):
		sourceType = AssetSourceDataURL
	case strings.HasPrefix(strings.ToLower(source), "http://"), strings.HasPrefix(strings.ToLower(source), "https://"):
	default:
		return "", false
	}

	renderer.imageIndex++

	assetDir := renderer.options.AssetDir
	if strings.TrimSpace(assetDir) == "" {
		assetDir = "assets"
	}

	prefix := strings.TrimSpace(renderer.options.AssetPrefix)
	if prefix == "" {
		prefix = "note"
	}

	extension := renderer.extensionForSource(source)
	fileName := fmt.Sprintf("%s-%03d%s", prefix, renderer.imageIndex, extension)
	relativePath := path.Join(assetDir, fileName)

	renderer.assets = append(renderer.assets, Asset{
		RelativePath: relativePath,
		SourceType:   sourceType,
		Source:       source,
	})

	return relativePath, true
}

func (renderer *renderer) extensionForSource(source string) string {
	if strings.HasPrefix(strings.ToLower(source), "data:") {
		metadata := strings.TrimPrefix(strings.ToLower(source), "data:")
		metadata, _, _ = strings.Cut(metadata, ",")
		parts := strings.Split(metadata, ";")
		if len(parts) > 0 {
			if extension := extensionForMediaType(parts[0]); extension != "" {
				return extension
			}
		}

		return ".bin"
	}

	parsedURL, err := url.Parse(source)
	if err != nil {
		return ".img"
	}

	extension := path.Ext(parsedURL.Path)
	if extension == "" {
		return ".img"
	}

	return extension
}

func (renderer *renderer) extractTextWithBreaks(node *html.Node) string {
	if node == nil {
		return ""
	}

	switch node.Type {
	case html.TextNode:
		return node.Data
	case html.ElementNode:
	default:
		return ""
	}

	if node.Data == "br" {
		return "\n"
	}

	var parts strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		parts.WriteString(renderer.extractTextWithBreaks(child))
		if child.Type == html.ElementNode && (child.Data == "div" || child.Data == "p") {
			parts.WriteString("\n")
		}
	}

	return parts.String()
}

func findElement(node *html.Node, tag string) *html.Node {
	if node == nil {
		return nil
	}

	if node.Type == html.ElementNode && node.Data == tag {
		return node
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findElement(child, tag); found != nil {
			return found
		}
	}

	return nil
}

func visit(node *html.Node, fn func(*html.Node)) {
	if node == nil {
		return
	}

	fn(node)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		visit(child, fn)
	}
}

func isBlockNode(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}

	switch node.Data {
	case "body", "article", "section", "div", "p", "h1", "h2", "h3", "ul", "ol", "li", "table", "blockquote", "pre", "object":
		return true
	default:
		return false
	}
}

func isMonospaceNode(node *html.Node) bool {
	style := strings.ToLower(attribute(node, "style"))
	return strings.Contains(style, "monospace") ||
		strings.Contains(style, "menlo") ||
		strings.Contains(style, "monaco") ||
		strings.Contains(style, "courier") ||
		strings.Contains(style, "sf mono") ||
		strings.Contains(style, "sfmono")
}

func isMonospaceBlock(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}

	if node.Data == "pre" {
		return true
	}

	if node.Data != "div" && node.Data != "p" && node.Data != "span" {
		return false
	}

	if isMonospaceNode(node) {
		return true
	}

	hasContent := false
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			if strings.TrimSpace(child.Data) != "" {
				return false
			}
		case html.ElementNode:
			if child.Data == "br" {
				hasContent = true
				continue
			}
			if !isMonospaceBlock(child) && !isMonospaceNode(child) {
				return false
			}
			hasContent = true
		}
	}

	return hasContent
}

func hasOnlyLineBreaks(node *html.Node) bool {
	if node == nil {
		return false
	}

	hasBreak := false
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			if strings.TrimSpace(child.Data) != "" {
				return false
			}
		case html.ElementNode:
			if child.Data == "br" {
				hasBreak = true
				continue
			}
			if !hasOnlyLineBreaks(child) {
				return false
			}
			hasBreak = true
		}
	}

	return hasBreak
}

func attribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return attribute.Val
		}
	}

	return ""
}

func hasTruthyAttribute(node *html.Node, name string) bool {
	for _, attribute := range node.Attr {
		if !strings.EqualFold(attribute.Key, name) {
			continue
		}

		if attribute.Val == "" {
			return true
		}

		value := strings.ToLower(strings.TrimSpace(attribute.Val))
		return value != "false" && value != "0"
	}

	return false
}

func compactWhitespace(value string) string {
	return whitespacePattern.ReplaceAllString(value, " ")
}

func normalizeParagraph(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	lines := strings.Split(value, "\n")
	var normalized []string

	for _, line := range lines {
		compacted := strings.TrimSpace(whitespacePattern.ReplaceAllString(line, " "))
		if compacted == "" {
			if len(normalized) == 0 || normalized[len(normalized)-1] == "" {
				continue
			}
			normalized = append(normalized, "")
			continue
		}

		normalized = append(normalized, compacted)
	}

	return strings.Join(normalized, "\n")
}

func normalizeCodeBlock(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimRight(line, " \t")
	}

	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

func normalizeMarkdown(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimRight(line, " \t")
	}

	value = strings.Join(lines, "\n")

	var normalized []string
	blankRun := 0
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) == "" {
			blankRun++
			if blankRun > 2 {
				continue
			}
			normalized = append(normalized, "")
			continue
		}

		blankRun = 0
		normalized = append(normalized, line)
	}

	value = strings.TrimSpace(strings.Join(normalized, "\n"))
	if value == "" {
		return ""
	}

	return value + "\n"
}

func formatListLines(indent string, marker string, text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	lines := strings.Split(text, "\n")
	formatted := make([]string, 0, len(lines))
	paddedIndent := indent + strings.Repeat(" ", len(marker))

	for index, line := range lines {
		if index == 0 {
			formatted = append(formatted, indent+marker+line)
			continue
		}
		formatted = append(formatted, paddedIndent+line)
	}

	return formatted
}

func prefixLines(value string, prefix string) string {
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[index] = strings.TrimRight(prefix, " ")
			continue
		}
		lines[index] = prefix + line
	}

	return strings.Join(lines, "\n")
}

func escapeTableCell(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func extensionForMediaType(mediaType string) string {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	extensions, err := mime.ExtensionsByType(mediaType)
	if err == nil && len(extensions) > 0 {
		return extensions[0]
	}

	switch mediaType {
	case "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/tiff":
		return ".tiff"
	default:
		return ""
	}
}

func escapeMarkdownText(value string) string {
	return markdownEscapeReplacer.Replace(value)
}
