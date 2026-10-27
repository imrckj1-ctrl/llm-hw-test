package exporter

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/richardlehane/mscfb"
)

// ole10Native 结构断言:对齐 WPS 样本(nUnknown2=3 + 三段 UTF-16 路径尾巴)
func TestOle10Native_Structure(t *testing.T) {
	data := []byte("PK\x03\x04hello")
	b := ole10Native("results.zip", data)
	if len(b) < 8 {
		t.Fatal("太短")
	}
	total := binary.LittleEndian.Uint32(b)
	if int(total) != len(b)-4 {
		t.Fatalf("totalSize=%d 应为 %d", total, len(b)-4)
	}
	if sig := binary.LittleEndian.Uint16(b[4:]); sig != 0x0002 {
		t.Fatalf("签名=0x%x", sig)
	}
	p := 6
	nextStr := func() string {
		end := bytes.IndexByte(b[p:], 0)
		s := string(b[p : p+end])
		p += end + 1
		return s
	}
	if l := nextStr(); l != "results.zip" {
		t.Fatalf("label=%q", l)
	}
	nextStr() // 源文件名
	if u1 := binary.LittleEndian.Uint16(b[p:]); u1 != 0 {
		t.Fatalf("nUnknown1=%d", u1)
	}
	if u2 := binary.LittleEndian.Uint16(b[p+2:]); u2 != 3 {
		t.Fatalf("nUnknown2=%d 应为 3(WPS 样本实测)", u2)
	}
	p += 4
	plen := int(binary.LittleEndian.Uint32(b[p:])); p += 4 + plen
	dlen := int(binary.LittleEndian.Uint32(b[p:])); p += 4
	if dlen != len(data) {
		t.Fatalf("dataLen=%d", dlen)
	}
	p += dlen
	// 三段 UTF-16 尾巴:字符数前缀 + 无终止符
	want := []string{`C:\Windows\Temp\results.zip`, "results.zip", "results.zip"}
	for i, w := range want {
		n := int(binary.LittleEndian.Uint32(b[p:])); p += 4
		u := make([]uint16, n)
		for j := range u {
			u[j] = binary.LittleEndian.Uint16(b[p+j*2:])
		}
		p += n * 2
		// 用 utf16 解码比对
		if got := runesOf(u); got != w {
			t.Fatalf("尾巴第%d段=%q 应为 %q", i, got, w)
		}
	}
	if p != len(b) {
		t.Fatalf("解析结束于 %d,总长 %d", p, len(b))
	}
}

func runesOf(u []uint16) string {
	return string(utf16Decode(u))
}

func utf16Decode(u []uint16) []rune {
	// 简单实现,测试用数据无代理对
	r := make([]rune, len(u))
	for i, c := range u {
		r[i] = rune(c)
	}
	return r
}

// 往返验证:buildCFBF 生成的复合存储必须能被标准 Reader 正确解析
func TestBuildCFBF_Roundtrip(t *testing.T) {
	cases := []struct {
		name    string
		streams []cfbStream
	}{
		{"三个小流", []cfbStream{
			{"\x01CompObj", bytes.Repeat([]byte{0x11}, 76)},
			{"\x01Ole10Native", bytes.Repeat([]byte{0x22}, 300)},
			{"\x03ObjInfo", []byte{0x40, 0x00, 0x03, 0x00, 0x01, 0x00}},
		}},
		{"小流+大流", []cfbStream{
			{"\x01CompObj", bytes.Repeat([]byte{0x11}, 76)},
			{"\x01Ole10Native", bytes.Repeat([]byte{0x33}, 70000)}, // 触发常规 FAT 链
			{"\x03ObjInfo", []byte{0x40, 0x00, 0x03, 0x00, 0x01, 0x00}},
		}},
		{"超大流触发DIFAT", []cfbStream{
			{"\x01Ole10Native", bytes.Repeat([]byte{0x44}, 8<<20)}, // 8MB, FAT 扇区超 109
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := buildCFBF(tc.streams, nil)
			if !bytes.HasPrefix(bin, []byte{0xD0, 0xCF, 0x11, 0xE0}) {
				t.Fatal("缺少 CFBF 签名")
			}
			r, err := mscfb.New(bytes.NewReader(bin))
			if err != nil {
				t.Fatalf("mscfb 无法解析: %v", err)
			}
			got := map[string][]byte{}
			for {
				e, err := r.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("枚举目录失败: %v", err)
				}
				data, err := io.ReadAll(e)
				if err != nil {
					t.Fatalf("读流 %q 失败: %v", e.Name, err)
				}
				got[e.Name] = data
			}
			for _, s := range tc.streams {
				// mscfb 会剥掉不可打印的首字符(\x01/\x03),比对时用剥掉后的名字
				name := s.name
				if name[0] < 0x20 {
					name = name[1:]
				}
				g, ok := got[name]
				if !ok {
					t.Fatalf("流 %q 未找到(已枚举: %v)", name, keysOf(got))
				}
				if !bytes.Equal(g, s.data) {
					t.Fatalf("流 %q 内容不一致: 得 %d 字节, 期望 %d 字节", name, len(g), len(s.data))
				}
			}
		})
	}
}

func keysOf(m map[string][]byte) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// olePackageBin 端到端:产出的 bin 必须是 CFBF 且 Ole10Native 流内含原始数据
func TestOlePackageBin(t *testing.T) {
	payload := bytes.Repeat([]byte("PK\x03\x04fakezip"), 100)
	bin, err := olePackageBin("results.zip", payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(bin, []byte{0xD0, 0xCF, 0x11, 0xE0}) {
		t.Fatal("产物不是 CFBF")
	}
	r, err := mscfb.New(bytes.NewReader(bin))
	if err != nil {
		t.Fatal(err)
	}
	// 根 storage 必须带 OLE Package CLSID(WPS 样基准值),否则 WPS 无法激活对象
	if id := r.ID(); id != "{0003000C-0000-0000-C000-000000000046}" {
		t.Fatalf("根 CLSID = %q, 期望 OLE Package CLSID", id)
	}
	for {
		e, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if e.Name == "Ole10Native" {
			data, _ := io.ReadAll(e)
			if !bytes.Contains(data, payload) {
				t.Fatal("Ole10Native 流内未找到原始数据")
			}
			return
		}
	}
	t.Fatal("未找到 Ole10Native 流")
}
