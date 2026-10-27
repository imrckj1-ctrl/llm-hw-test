// 迷你 DOCX 生成器(零依赖):docx 即 zip+XML。版式对齐《大模型推理硬件性能测试报告-模板》样例 v4:
// 封面/目录/蓝色标题(2E74B5)/灰底表头(F2F4F7)/居中小字表注/说明框,支持 OLE 附件嵌入(双击打开原文件)
package exporter

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// 预渲染的 OLE 附件图标(PIL 绘制,类 Word 手动插入样式)
//
//go:embed assets/ole_zip.png
var iconZip []byte

//go:embed assets/ole_xlsx.png
var iconXlsx []byte

//go:embed assets/ole_pptx.png
var iconPptx []byte

//go:embed assets/ole_docx.png
var iconDocx []byte

//go:embed assets/ole_pdf.png
var iconPdf []byte

//go:embed assets/ole_html.png
var iconHtml []byte

//go:embed assets/echarts.min.js
var echartsJS []byte

type oleAttachment struct {
	rid  string
	path string // word/embeddings/oleObjectN.ext
	ext  string
	data []byte
}

// ole10Native 包对象封装(WPS/Word 打开 zip/pdf 等无专属 ProgID 附件必须此格式)。
// 结构逐字段对齐 WPS 原生插入产物(wps附件样本.docx 实测 dump)与 LibreOffice 解析逻辑:
//   uint16 0x0002 | label\0 | 源文件名\0 | uint16 0 | uint16 3 | ANSI路径长度+路径\0 |
//   数据长度+数据 | 三段「uint32 字符数 + UTF-16LE(无终止)」: 临时路径/文件名/源路径
// 缺 nUnknown2=3 或 Unicode 尾巴时,WPS 双击会走降级路径报「无法打开此对象」
func ole10Native(filename string, data []byte) []byte {
	// 非 ASCII 文件名换成安全名(ANSI 标签)
	isASCII := func(s string) bool {
		for _, r := range s {
			if r > 127 {
				return false
			}
		}
		return true
	}
	label := filename
	if !isASCII(label) {
		ext := filepath.Ext(filename)
		label = "attachment" + ext
	}
	tmpPath := `C:\Windows\Temp\` + label // 提取建议路径(不存在不影响激活)
	payload := &bytes.Buffer{}
	binary.Write(payload, binary.LittleEndian, uint16(0x0002)) // OLE10 签名
	payload.WriteString(label + "\x00")                        // 标签
	payload.WriteString(label + "\x00")                        // 源文件名
	binary.Write(payload, binary.LittleEndian, uint16(0))      // nUnknown1
	binary.Write(payload, binary.LittleEndian, uint16(3))      // nUnknown2(样本实测=3)
	binary.Write(payload, binary.LittleEndian, uint32(len(tmpPath)+1))
	payload.WriteString(tmpPath + "\x00")
	binary.Write(payload, binary.LittleEndian, uint32(len(data)))
	payload.Write(data)
	// Unicode 尾巴:临时路径 / 文件名 / 源路径(长度前缀为字符数,无终止符)
	for _, s := range []string{tmpPath, label, label} {
		u := utf16.Encode([]rune(s))
		binary.Write(payload, binary.LittleEndian, uint32(len(u)))
		for _, c := range u {
			binary.Write(payload, binary.LittleEndian, c)
		}
	}
	// 头 4 字节 = 其后全部内容长度
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, uint32(payload.Len()))
	buf.Write(payload.Bytes())
	return buf.Bytes()
}

// WPS 实测:Package 附件的 .bin 必须是 CFBF 复合存储(D0CF11E0),
// 内含 \x01CompObj / \x01Ole10Native / \x03ObjInfo 三个流;平面 OLE10Native 打不开
var compObjPackage = []byte{
	0x01, 0x00, 0xfe, 0xff, 0x03, 0x0a, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0x0c, 0x00, 0x03, 0x00,
	0x00, 0x00, 0x00, 0x00, 0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46, 0x0c, 0x00, 0x00, 0x00,
	0x4f, 0x4c, 0x45, 0x20, 0x50, 0x61, 0x63, 0x6b, 0x61, 0x67, 0x65, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x08, 0x00, 0x00, 0x00, 0x50, 0x61, 0x63, 0x6b, 0x61, 0x67, 0x65, 0x00, 0xf4, 0x39, 0xb2, 0x71,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

var objInfoPackage = []byte{0x40, 0x00, 0x03, 0x00, 0x01, 0x00}

// olePackageBin 把 OLE10Native 装入 CFBF 复合存储(与 WPS 附件样本逐流一致)
func olePackageBin(filename string, data []byte) ([]byte, error) {
	ole10 := ole10Native(filename, data)
	return buildCFBF([]cfbStream{
		{"\x01CompObj", compObjPackage},
		{"\x01Ole10Native", ole10},
		{"\x03ObjInfo", objInfoPackage},
	}, olePackageCLSID), nil
}

type docx struct {
	body       strings.Builder
	atts       []oleAttachment
	extraRels  []string // document.xml.rels 追加项
	ridN       int      // rId 计数器
	iconRids   map[string]string // 图标类型 → 图片 rel(惰性登记)
	iconFiles  map[string][]byte // 图标类型 → PNG 内容(打包时写入)
	captionN   int      // 表注序号
	usedExts   map[string]bool
	tocAnchors map[string]string // 章节标题 → 书签锚点(可点击目录)
	bookmarkID int               // 书签 id 计数器(必须全局唯一,重复时 WPS/Word 忽略书签)
}

func newDocx() *docx {
	return &docx{ridN: 1, usedExts: map[string]bool{}, iconRids: map[string]string{}, iconFiles: map[string][]byte{}, tocAnchors: map[string]string{}}
}

// RegisterChapters 预注册章节锚点(目录先写、正文后写,顺序必须一致)
func (d *docx) RegisterChapters(titles []string) {
	for i, t := range titles {
		d.tocAnchors[t] = fmt.Sprintf("_Toc%d", i+1)
	}
}

func esc(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (d *docx) nextRid() string {
	r := fmt.Sprintf("rId%d", d.ridN)
	d.ridN++
	return r
}

// run 一段文字:eastAsia 中文字体 + 字号(半点) + 加粗 + 颜色
func run(text, eastAsia string, sz int, bold bool, color string) string {
	b, c := "", ""
	if bold {
		b = "<w:b/>"
	}
	if color != "" {
		c = fmt.Sprintf(`<w:color w:val="%s"/>`, color)
	}
	return fmt.Sprintf(`<w:r><w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:eastAsia="%s"/>%s%s<w:sz w:val="%d"/><w:szCs w:val="%d"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r>`,
		eastAsia, b, c, sz, sz, esc(text))
}

// para 段落:before/after 为 twips(w:spacing 原值)
func para(jc string, before, after int, runs ...string) string {
	j := ""
	if jc != "" {
		j = fmt.Sprintf(`<w:jc w:val="%s"/>`, jc)
	}
	return fmt.Sprintf(`<w:p><w:pPr><w:spacing w:before="%d" w:after="%d" w:line="360" w:lineRule="auto"/>%s</w:pPr>%s</w:p>`,
		before, after, j, strings.Join(runs, ""))
}

// ---------- 封面 ----------

func (d *docx) CoverTitle(text string) {
	d.body.WriteString(para("", 2400, 200, run(text, "微软雅黑", 46, true, "0B2545")))
}

func (d *docx) CoverSubtitle(text string) {
	d.body.WriteString(para("", 0, 600, run(text, "微软雅黑", 26, false, "555555")))
}

func (d *docx) CoverInfo(label, value string) {
	d.body.WriteString(para("", 0, 160,
		run(label+": ", "微软雅黑", 22, true, "1A1A1A"),
		run(value, "微软雅黑", 22, false, "666666")))
}

// ---------- 目录 ----------

func (d *docx) TOCTitle() {
	d.body.WriteString(para("", 400, 200, run("目  录", "微软雅黑", 26, true, "1F3A5F")))
}

func (d *docx) TOCEntry(text string) {
	anchor := d.tocAnchors[text]
	if anchor == "" {
		d.body.WriteString(para("", 0, 80, run(text, "微软雅黑", 21, false, "333333")))
		return
	}
	// 已注册章节:内部超链接,点击跳转到对应章标题(Word 目录样式:纯文本不画线)
	d.body.WriteString(para("", 0, 80,
		fmt.Sprintf(`<w:hyperlink w:anchor="%s" w:history="1">%s</w:hyperlink>`, anchor, run(text, "微软雅黑", 21, false, "333333"))))
}

// ---------- 标题层级(模板:Word 蓝 2E74B5) ----------

func (d *docx) H1(text string) {
	d.body.WriteString(para("", 320, 160, run(text, "微软雅黑", 32, true, "2E74B5")))
}

// H1B 章标题(带书签锚点,供目录点击跳转);未注册的标题退化为普通 H1
func (d *docx) H1B(text string) {
	anchor := d.tocAnchors[text]
	if anchor == "" {
		d.H1(text)
		return
	}
	d.bookmarkID++
	bmID := 200 + d.bookmarkID
	d.body.WriteString(fmt.Sprintf(`<w:p><w:pPr><w:spacing w:before="320" w:after="160" w:line="360" w:lineRule="auto"/></w:pPr>`+
		`<w:bookmarkStart w:id="%d" w:name="%s"/>%s<w:bookmarkEnd w:id="%d"/></w:p>`,
		bmID, anchor, run(text, "微软雅黑", 32, true, "2E74B5"), bmID))
}

func (d *docx) H2(text string) {
	d.body.WriteString(para("", 240, 120, run(text, "微软雅黑", 26, true, "2E74B5")))
}

func (d *docx) H3(text string) {
	d.body.WriteString(para("", 200, 60, run(text, "微软雅黑", 22, true, "2E74B5")))
}

// P 正文段落(宋体 10.5pt);color 可空
func (d *docx) P(text string) {
	d.body.WriteString(para("", 0, 120, run(text, "宋体", 21, false, "")))
}

func (d *docx) PColor(text, color string) {
	d.body.WriteString(para("", 0, 120, run(text, "宋体", 21, false, color)))
}

// BlankLines 留白(测试结论手写区)
func (d *docx) BlankLines(n int) {
	for i := 0; i < n; i++ {
		d.body.WriteString(para("", 0, 360))
	}
}

// Caption 表注(自动编号,居中 9pt 灰)
func (d *docx) Caption(text string) {
	d.captionN++
	d.body.WriteString(para("center", 60, 200,
		run(fmt.Sprintf("表 %d %s", d.captionN, text), "微软雅黑", 18, false, "595959")))
}

// PageBreak 分页
func (d *docx) PageBreak() {
	d.body.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
}

// ---------- 表格 ----------

// Table 表格:表头灰底 F2F4F7 加粗,全边框,总宽 9360 dxa(模板页宽)
func (d *docx) Table(headers []string, rows [][]string) {
	var b strings.Builder
	ncol := len(headers)
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="9360" w:type="dxa"/><w:tblLayout w:type="fixed"/><w:tblBorders>`)
	for _, side := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		b.WriteString(fmt.Sprintf(`<w:%s w:val="single" w:sz="4" w:space="0" w:color="auto"/>`, side))
	}
	b.WriteString(`</w:tblBorders><w:tblCellMar><w:top w:w="40" w:type="dxa"/><w:bottom w:w="40" w:type="dxa"/><w:left w:w="80" w:type="dxa"/><w:right w:w="80" w:type="dxa"/></w:tblCellMar></w:tblPr>`)
	b.WriteString("<w:tblGrid>")
	for i := 0; i < ncol; i++ {
		b.WriteString(fmt.Sprintf(`<w:gridCol w:w="%d"/>`, 9360/ncol))
	}
	b.WriteString("</w:tblGrid>")
	cell := func(text string, header bool) {
		shd, bold := "", false
		if header {
			shd = `<w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/>`
			bold = true
		}
		align := "center"
		if utf8.RuneCountInString(text) > 14 { // 长文本左对齐
			align = ""
		}
		fmt.Fprintf(&b, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>%s<w:vAlign w:val="center"/></w:tcPr>%s</w:tc>`,
			9360/ncol, shd, para(align, 20, 20, run(text, "宋体", 18, bold, "")))
	}
	b.WriteString("<w:tr>")
	for _, h := range headers {
		cell(h, true)
	}
	b.WriteString("</w:tr>")
	for _, row := range rows {
		b.WriteString("<w:tr>")
		for i := 0; i < ncol; i++ {
			v := ""
			if i < len(row) {
				v = row[i]
			}
			cell(v, false)
		}
		b.WriteString("</w:tr>")
	}
	b.WriteString("</w:tbl>")
	d.body.WriteString(b.String())
}

// NoteBox 说明框(1×1 灰底表格,模板中的测试方法/提示块)
func (d *docx) NoteBox(text string) {
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="9360" w:type="dxa"/><w:tblLayout w:type="fixed"/><w:tblBorders>`)
	for _, side := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		b.WriteString(fmt.Sprintf(`<w:%s w:val="single" w:sz="4" w:space="0" w:color="auto"/>`, side))
	}
	b.WriteString(`</w:tblBorders><w:tblCellMar><w:top w:w="60" w:type="dxa"/><w:bottom w:w="60" w:type="dxa"/><w:left w:w="100" w:type="dxa"/><w:right w:w="100" w:type="dxa"/></w:tblCellMar></w:tblPr>`)
	b.WriteString(`<w:tblGrid><w:gridCol w:w="9360"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:tcW w:w="9360" w:type="dxa"/><w:shd w:val="clear" w:color="auto" w:fill="F7F8FA"/></w:tcPr>`)
	b.WriteString(para("", 20, 20, run(text, "宋体", 18, false, "444444")))
	b.WriteString(`</w:tc></w:tr></w:tbl>`)
	d.body.WriteString(b.String())
	d.body.WriteString(para("", 0, 120))
}

// ---------- OLE 附件嵌入 ----------

var oleProgID = map[string]string{
	"xlsx": "Excel.Sheet.12",
	"docx": "Word.Document.12",
	"pptx": "PowerPoint.Show.12",
	"pdf":  "Package",
	"zip":  "Package",
}

var oleIcon = map[string]struct {
	key  string
	data []byte
}{
	"xlsx": {"xlsx", iconXlsx},
	"pptx": {"pptx", iconPptx},
	"docx": {"docx", iconDocx},
	"pdf":  {"pdf", iconPdf},
	"html": {"html", iconHtml},
	"zip":  {"zip", iconZip},
}

// Attach 嵌入文件对象:说明行 + 类型图标(图标下方挂文件名,类 Word 手动插入;双击打开)
func (d *docx) Attach(title, filename, ext string, data []byte) {
	progid := oleProgID[ext]
	if progid == "" {
		progid = "Package"
	}
	ico := oleIcon[ext]
	if ico.data == nil {
		ico = oleIcon["zip"]
	}
	d.body.WriteString(para("", 0, 40,
		run(fmt.Sprintf("附件:%s(%s,双击图标打开)", title, filename), "宋体", 18, false, "595959")))
	imgRid := d.iconRids[ico.key]
	if imgRid == "" {
		imgRid = d.nextRid()
		d.iconRids[ico.key] = imgRid
		d.iconFiles[ico.key] = ico.data
		d.extraRels = append(d.extraRels, fmt.Sprintf(`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/ole_%s.png"/>`, imgRid, ico.key))
	}
	rid := d.nextRid()
	idx := len(d.atts) + 1
	storeExt := ext
	payload := data
	if progid == "Package" {
		// 包对象需 CFBF 复合存储封装(对齐 WPS 原生附件样本);失败降级 OLE10Native 平面封装
		if pkg, err := olePackageBin(filename, data); err == nil {
			payload = pkg
		} else {
			payload = ole10Native(filename, data)
		}
		storeExt = "bin"
	}
	store := fmt.Sprintf("embeddings/oleObject%d.%s", idx, storeExt)
	d.extraRels = append(d.extraRels, fmt.Sprintf(`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/oleObject" Target="%s"/>`, rid, store))
	d.atts = append(d.atts, oleAttachment{rid: rid, path: "word/" + store, ext: storeExt, data: payload})
	if storeExt == "bin" {
		d.usedExts["bin"] = true
	} else {
		d.usedExts[ext] = true
	}
	// 对象 XML 逐字段对齐 WPS 原生插入结构(实测基准:wps附件样本.docx):
	// 无 shapetype 定义、shape 带 o:spt/o:ole="t"/w10:wrap、OLEObject 带 LockedField=false
	shapeID := 1025 + idx
	objectID := 1468075700 + idx
	d.body.WriteString(fmt.Sprintf(`<w:p><w:pPr><w:spacing w:after="120"/></w:pPr><w:r><w:object>`+
		`<v:shape id="_x0000_i%d" o:spt="75" type="#_x0000_t75" style="height:73pt;width:64pt;" o:ole="t" filled="f" o:preferrelative="t" stroked="f" coordsize="21600,21600">`+
		`<v:fill on="f" focussize="0,0"/><v:stroke on="f"/>`+
		`<v:imagedata r:id="%s" o:title=""/>`+
		`<o:lock v:ext="edit" aspectratio="t"/><w10:wrap type="none"/><w10:anchorlock/></v:shape>`+
		`<o:OLEObject Type="Embed" ProgID="%s" ShapeID="_x0000_i%d" DrawAspect="Icon" ObjectID="_%d" r:id="%s">`+
		`<o:LockedField>false</o:LockedField></o:OLEObject></w:object></w:r></w:p>`,
		shapeID, imgRid, progid, shapeID, objectID, rid))
	// 图标下方挂文件名(Word 手动插入的样式)
	d.body.WriteString(para("", 0, 120, run(filename, "宋体", 16, false, "404040")))
}

var oleContentType = map[string]string{
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"pdf":  "application/pdf",
	"zip":  "application/zip",
	"bin":  "application/vnd.openxmlformats-officedocument.oleObject",
	"png":  "image/png",
}

// 页脚:正文页居中「第 X 页 / 共 Y 页」;封面页(titlePg)页脚留空
var footerPageNum = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:jc w:val="center"/></w:pPr>` +
	`<w:r><w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:eastAsia="宋体"/><w:sz w:val="18"/></w:rPr><w:t xml:space="preserve">第 </w:t></w:r>` +
	`<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> PAGE </w:instrText></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r>` +
	`<w:r><w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:eastAsia="宋体"/><w:sz w:val="18"/></w:rPr><w:t xml:space="preserve"> 页 / 共 </w:t></w:r>` +
	`<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> NUMPAGES </w:instrText></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r>` +
	`<w:r><w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:eastAsia="宋体"/><w:sz w:val="18"/></w:rPr><w:t xml:space="preserve"> 页</w:t></w:r>` +
	`</w:p></w:ftr>`

var footerBlank = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p/></w:ftr>`

// Build 打包 docx:模板页面 12240×15840 twips,四边距 1 英寸
func (d *docx) Build() (*bytes.Buffer, error) {
	ftrRidDef := d.nextRid()   // 默认页脚(页码)
	ftrRidFirst := d.nextRid() // 首页页脚(空白,封面不显示页码)
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
		`xmlns:w10="urn:schemas-microsoft-com:office:word" ` +
		`xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office"><w:body>` +
		d.body.String() +
		`<w:sectPr>` +
		fmt.Sprintf(`<w:footerReference w:type="default" r:id="%s"/><w:footerReference w:type="first" r:id="%s"/>`, ftrRidDef, ftrRidFirst) +
		`<w:titlePg/>` +
		`<w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="708" w:footer="708" w:gutter="0"/></w:sectPr></w:body></w:document>`
	// 页脚关系放 extraRels 最前(document.xml.rels)
	d.extraRels = append([]string{
		fmt.Sprintf(`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>`, ftrRidDef),
		fmt.Sprintf(`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer2.xml"/>`, ftrRidFirst),
	}, d.extraRels...)

	ctypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>`
	for ext := range d.usedExts {
		if ct, ok := oleContentType[ext]; ok {
			ctypes += fmt.Sprintf(`<Default Extension="%s" ContentType="%s"/>`, ext, ct)
		}
	}
	if len(d.iconRids) > 0 {
		ctypes += `<Default Extension="png" ContentType="image/png"/>`
	}
	ctypes += `<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
		`<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>` +
		`<Override PartName="/word/footer2.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/></Types>`

	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		strings.Join(d.extraRels, "") + `</Relationships>`

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	write := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}
	if err := write("[Content_Types].xml", ctypes); err != nil {
		return nil, err
	}
	rootRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
		`</Relationships>`
	if err := write("_rels/.rels", rootRels); err != nil {
		return nil, err
	}
	if err := write("word/document.xml", document); err != nil {
		return nil, err
	}
	if err := write("word/_rels/document.xml.rels", rels); err != nil {
		return nil, err
	}
	if err := write("word/footer1.xml", footerPageNum); err != nil {
		return nil, err
	}
	if err := write("word/footer2.xml", footerBlank); err != nil {
		return nil, err
	}
	for key, data := range d.iconFiles {
		w, err := zw.Create("word/media/ole_" + key + ".png")
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	for _, att := range d.atts {
		w, err := zw.Create(att.path)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(att.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}
