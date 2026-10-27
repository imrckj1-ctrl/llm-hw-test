package exporter

import (
	"bytes"
	"encoding/binary"
	"sort"
	"unicode/utf16"
)

// 最小 CFBF(复合文件二进制格式)写入器,major v3 / 512 字节扇区。
// 用途:OLE Package 附件(zip/pdf 等)必须以 CFBF 存储嵌入,WPS/Word 才能双击打开。
// 结构:头部 + [目录扇区][mini stream 扇区][大流扇区][MiniFAT 扇区][FAT 扇区][DIFAT 扇区]
// 小流(<4096 字节)走 64 字节 mini 扇区,经 MiniFAT 链接;大流走常规 FAT 链。

const (
	cfbSectorSize  = 512
	cfbMiniSize    = 64
	cfbMiniCutoff  = 4096
	cfbFree        = 0xFFFFFFFF
	cfbEndOfChain  = 0xFFFFFFFE
	cfbFATSect     = 0xFFFFFFFD
	cfbDIFATSect   = 0xFFFFFFFC
	cfbNoStream    = -1 // 目录项 int32 字段的"无"值(即 0xFFFFFFFF)
)

type cfbStream struct {
	name string // 流名(会转 UTF-16,需 <= 31 字符)
	data []byte
}

type cfbEntry struct {
	name  string
	data  []byte
	isRoot bool
	mini  bool   // 是否走 mini stream
	start uint32 // 起始扇区(mini 流为 mini 扇区号)
	child, left, right int32
}

// cfbNameCmp OLE 目录排序规则:先按名字长度,再按大写码点
func cfbNameCmp(a, b string) int {
	ra, rb := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	if len(ra) != len(rb) {
		return len(ra) - len(rb)
	}
	for i := range ra {
		ca, cb := ra[i], rb[i]
		if ca >= 'a' && ca <= 'z' {
			ca -= 32
		}
		if cb >= 'a' && cb <= 'z' {
			cb -= 32
		}
		if ca != cb {
			return int(ca) - int(cb)
		}
	}
	return 0
}

// buildDirTree 按排序后的索引构造平衡二叉树,返回根在 entries 中的下标
func buildDirTree(entries []cfbEntry, sorted []int) int32 {
	if len(sorted) == 0 {
		return cfbNoStream
	}
	mid := len(sorted) / 2
	root := sorted[mid]
	entries[root].left = buildDirTree(entries, sorted[:mid])
	entries[root].right = buildDirTree(entries, sorted[mid+1:])
	return int32(root)
}

func pad(buf *bytes.Buffer, unit int) {
	if r := buf.Len() % unit; r != 0 {
		buf.Write(make([]byte, unit-r))
	}
}

func putU32(b []byte, v uint32) { binary.LittleEndian.PutUint32(b, v) }

// olePackageCLSID OLE Package 对象的注册 CLSID {0003000C-0000-0000-C000-000000000046},
// 必须写入根 storage 的目录项,否则 WPS/Word 双击时找不到激活 server,报「无法打开此对象」
var olePackageCLSID = []byte{
	0x0C, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46,
}

// buildCFBF 把若干流打包成一个 CFBF 文件;rootCLSID 非空时写入根目录项(16 字节)
func buildCFBF(streams []cfbStream, rootCLSID []byte) []byte {
	entries := make([]cfbEntry, 0, len(streams)+1)
	entries = append(entries, cfbEntry{name: "Root Entry", isRoot: true, left: cfbNoStream, right: cfbNoStream, child: cfbNoStream})
	for _, s := range streams {
		entries = append(entries, cfbEntry{
			name: s.name, data: s.data,
			mini:  len(s.data) < cfbMiniCutoff,
			left:  cfbNoStream, right: cfbNoStream, child: cfbNoStream,
		})
	}
	// 目录兄弟树:按 OLE 规则排序后建平衡树
	sorted := make([]int, len(streams))
	for i := range sorted {
		sorted[i] = i + 1
	}
	sort.Slice(sorted, func(a, b int) bool {
		return cfbNameCmp(entries[sorted[a]].name, entries[sorted[b]].name) < 0
	})
	entries[0].child = buildDirTree(entries, sorted)

	// mini stream:小流顺序拼接,各自 pad 到 64
	mini := &bytes.Buffer{}
	for i := range entries {
		e := &entries[i]
		if e.isRoot || !e.mini {
			continue
		}
		e.start = uint32(mini.Len() / cfbMiniSize)
		mini.Write(e.data)
		pad(mini, cfbMiniSize)
	}
	miniBytes := mini.Bytes()

	// 扇区布局计数
	dirSects := (len(entries) + 3) / 4 // 每扇区 4 个目录项
	miniSects := (len(miniBytes) + cfbSectorSize - 1) / cfbSectorSize
	miniFATEntries := len(miniBytes) / cfbMiniSize
	miniFATSects := (miniFATEntries + 127) / 128
	if miniFATEntries == 0 {
		miniFATSects = 0
	}
	// 大流扇区数
	bigSects := 0
	for i := range entries {
		e := &entries[i]
		if e.isRoot || e.mini {
			continue
		}
		n := (len(e.data) + cfbSectorSize - 1) / cfbSectorSize
		e.start = 0 // 先占位,布局阶段赋值
		bigSects += n
		_ = n
	}
	// FAT 扇区数:FAT 项 = dir + miniStream + big + miniFAT + fat 自身
	base := dirSects + miniSects + bigSects + miniFATSects
	fatSects := 1
	for {
		need := (base + fatSects + 127) / 128
		if need == fatSects {
			break
		}
		fatSects = need
	}
	// DIFAT:头部仅 109 槽,超出走 DIFAT 扇区(每扇区 127 个 FAT 引用)
	difatSects := 0
	if fatSects > 109 {
		difatSects = (fatSects - 109 + 126) / 127
	}

	// 分配扇区号:dir → miniStream → 大流 → MiniFAT → FAT → DIFAT
	var sec uint32
	firstDir := sec
	sec += uint32(dirSects)
	firstMiniStream := uint32(cfbEndOfChain)
	if miniSects > 0 {
		firstMiniStream = sec
		sec += uint32(miniSects)
	}
	for i := range entries {
		e := &entries[i]
		if e.isRoot || e.mini {
			continue
		}
		e.start = sec
		sec += uint32((len(e.data) + cfbSectorSize - 1) / cfbSectorSize)
	}
	firstMiniFAT := uint32(cfbEndOfChain)
	if miniFATSects > 0 {
		firstMiniFAT = sec
		sec += uint32(miniFATSects)
	}
	firstFAT := sec
	sec += uint32(fatSects)
	firstDIFAT := uint32(cfbEndOfChain)
	if difatSects > 0 {
		firstDIFAT = sec
		sec += uint32(difatSects)
	}
	totalSects := int(sec)

	// FAT 表
	fat := make([]uint32, totalSects-difatSects)
	for i := range fat {
		fat[i] = cfbFree
	}
	// dir 链
	for i := 0; i < dirSects; i++ {
		if i+1 < dirSects {
			fat[firstDir+uint32(i)] = firstDir + uint32(i) + 1
		} else {
			fat[firstDir+uint32(i)] = cfbEndOfChain
		}
	}
	// mini stream 链
	for i := 0; i < miniSects; i++ {
		if i+1 < miniSects {
			fat[firstMiniStream+uint32(i)] = firstMiniStream + uint32(i) + 1
		} else {
			fat[firstMiniStream+uint32(i)] = cfbEndOfChain
		}
	}
	// 大流链
	for i := range entries {
		e := &entries[i]
		if e.isRoot || e.mini {
			continue
		}
		n := (len(e.data) + cfbSectorSize - 1) / cfbSectorSize
		for j := 0; j < n; j++ {
			if j+1 < n {
				fat[e.start+uint32(j)] = e.start + uint32(j) + 1
			} else {
				fat[e.start+uint32(j)] = cfbEndOfChain
			}
		}
	}
	// MiniFAT 链
	for i := 0; i < miniFATSects; i++ {
		if i+1 < miniFATSects {
			fat[firstMiniFAT+uint32(i)] = firstMiniFAT + uint32(i) + 1
		} else {
			fat[firstMiniFAT+uint32(i)] = cfbEndOfChain
		}
	}
	// FAT 扇区自身
	for i := 0; i < fatSects; i++ {
		fat[firstFAT+uint32(i)] = cfbFATSect
	}

	// MiniFAT 表:按 mini 扇区顺序串各小流
	miniFAT := make([]uint32, miniFATSects*128)
	for i := range miniFAT {
		miniFAT[i] = cfbFree
	}
	for i := range entries {
		e := &entries[i]
		if e.isRoot || !e.mini {
			continue
		}
		n := (len(e.data) + cfbMiniSize - 1) / cfbMiniSize
		for j := 0; j < n; j++ {
			if j+1 < n {
				miniFAT[e.start+uint32(j)] = e.start + uint32(j) + 1
			} else {
				miniFAT[e.start+uint32(j)] = cfbEndOfChain
			}
		}
	}

	// 拼装文件体
	out := &bytes.Buffer{}
	out.Write(make([]byte, cfbSectorSize)) // 头部占位,最后回填

	writeSect := func(fill func(b []byte)) {
		b := make([]byte, cfbSectorSize)
		if fill != nil {
			fill(b)
		}
		out.Write(b)
	}

	// 目录扇区
	for s := 0; s < dirSects; s++ {
		from, to := s*4, (s+1)*4
		if to > len(entries) {
			to = len(entries)
		}
		group := entries[from:to]
		writeSect(func(b []byte) {
			for j, e := range group {
				off := j * 128
				u := utf16.Encode([]rune(e.name))
				for k, c := range u {
					binary.LittleEndian.PutUint16(b[off+k*2:], c)
				}
				binary.LittleEndian.PutUint16(b[off+64:], uint16((len(u)+1)*2))
				typ := byte(2)
				if e.isRoot {
					typ = 5
				}
				b[off+66] = typ
				b[off+67] = 1 // black
				putU32(b[off+68:], uint32(e.left))
				putU32(b[off+72:], uint32(e.right))
				putU32(b[off+76:], uint32(e.child))
				if e.isRoot && len(rootCLSID) == 16 {
					copy(b[off+80:off+96], rootCLSID)
				}
				start := e.start
				size := uint64(len(e.data))
				if e.isRoot {
					start = firstMiniStream
					size = uint64(len(miniBytes))
				}
				putU32(b[off+116:], start)
				binary.LittleEndian.PutUint64(b[off+120:], size)
			}
		})
	}
	// mini stream 扇区
	for i := 0; i < len(miniBytes); i += cfbSectorSize {
		end := i + cfbSectorSize
		if end > len(miniBytes) {
			end = len(miniBytes)
		}
		chunk := miniBytes[i:end]
		writeSect(func(b []byte) { copy(b, chunk) })
	}
	// 大流扇区
	for i := range entries {
		e := &entries[i]
		if e.isRoot || e.mini {
			continue
		}
		data := e.data
		for j := 0; j < len(data); j += cfbSectorSize {
			end := j + cfbSectorSize
			if end > len(data) {
				end = len(data)
			}
			chunk := data[j:end]
			writeSect(func(b []byte) { copy(b, chunk) })
		}
	}
	// MiniFAT 扇区
	for i := 0; i < len(miniFAT); i += 128 {
		end := i + 128
		if end > len(miniFAT) {
			end = len(miniFAT)
		}
		chunk := miniFAT[i:end]
		writeSect(func(b []byte) {
			for j, v := range chunk {
				putU32(b[j*4:], v)
			}
		})
	}
	// FAT 扇区
	for i := 0; i < len(fat); i += 128 {
		end := i + 128
		if end > len(fat) {
			end = len(fat)
		}
		chunk := fat[i:end]
		writeSect(func(b []byte) {
			for j, v := range chunk {
				putU32(b[j*4:], v)
			}
		})
	}
	// DIFAT 扇区
	if difatSects > 0 {
		for d := 0; d < difatSects; d++ {
			dd := d
			writeSect(func(b []byte) {
				base := 109 + dd*127
				for j := 0; j < 127; j++ {
					idx := base + j
					if idx < fatSects {
						putU32(b[j*4:], firstFAT+uint32(idx))
					} else {
						putU32(b[j*4:], cfbFree)
					}
				}
				if dd+1 < difatSects {
					putU32(b[127*4:], firstDIFAT+uint32(dd)+1)
				} else {
					putU32(b[127*4:], cfbEndOfChain)
				}
			})
		}
	}

	// 回填头部
	head := out.Bytes()[:cfbSectorSize]
	copy(head, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	binary.LittleEndian.PutUint16(head[24:], 0x003E) // minor
	binary.LittleEndian.PutUint16(head[26:], 0x0003) // major v3
	binary.LittleEndian.PutUint16(head[28:], 0xFFFE) // byte order
	binary.LittleEndian.PutUint16(head[30:], 9)      // sector shift
	binary.LittleEndian.PutUint16(head[32:], 6)      // mini sector shift
	putU32(head[40:], 0)                             // numDirSectors(v3 固定 0)
	putU32(head[44:], uint32(fatSects))
	putU32(head[48:], firstDir)
	putU32(head[56:], cfbMiniCutoff)
	putU32(head[60:], firstMiniFAT)
	putU32(head[64:], uint32(miniFATSects))
	putU32(head[68:], firstDIFAT)
	putU32(head[72:], uint32(difatSects))
	for i := 0; i < 109; i++ {
		if i < fatSects {
			putU32(head[76+i*4:], firstFAT+uint32(i))
		} else {
			putU32(head[76+i*4:], cfbFree)
		}
	}

	return out.Bytes()
}
