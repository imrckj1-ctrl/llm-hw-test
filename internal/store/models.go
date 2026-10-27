package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// ---------- 任务参数快照(§5.5) ----------

type GenParams struct {
	Temperature float64 `json:"temperature"`
	TopP        float64 `json:"top_p"`
	TopK        int     `json:"top_k"`
	MaxTokens   int     `json:"max_tokens"`
}

type ExecParams struct {
	Warmup               int      `json:"warmup"`
	Repeats              int      `json:"repeats"`
	LadderTiers          []string `json:"ladder_tiers"`
	ConcurrencyLevels    []int    `json:"concurrency_levels"`
	StabilityMinutes     int      `json:"stability_minutes"`
	StabilityTier        string   `json:"stability_tier"`
	StabilityConcurrency int      `json:"stability_concurrency"`
	// V2.3:分项重复次数(0=用 Repeats)
	LadderRepeats      int `json:"ladder_repeats"`
	ConcurrencyRepeats int `json:"concurrency_repeats"`
	CacheRepeats       int `json:"cache_repeats"`
	// V2.5:NIAH 检索测试(深度×档位矩阵)
	NIAHTiers   []string `json:"niah_tiers"`   // 空=跟随阶梯档位
	NIAHDepths  []int    `json:"niah_depths"`  // 埋针深度百分比,默认 [10 30 50 70 90]
	NIAHRepeats int      `json:"niah_repeats"` // 每格重复,默认 2
	// V2.7:NIAH 重构为「有效上下文验证」(宣称值百分比扫描,1% 步进)
	NIAHClaimedK int `json:"niah_claimed_k"` // 宣称上下文(K),>0 走新逻辑
	NIAHStepPct  int `json:"niah_step_pct"`  // 扫描步进 %,默认 1
	NIAHDepthPct int `json:"niah_depth_pct"` // 埋针深度 %,默认 50
	// V2.13:未填上下文时探测模式上限(K),0=默认 1000K(百万级)
	NIAHMaxK int `json:"niah_max_k"`
	// V2.6:并发输入档(默认 1K)+ 缓存测试档位(空=跟随阶梯档位)
	ConcurrencyTier string   `json:"concurrency_tier"`
	CacheTiers      []string `json:"cache_tiers"`
	// V2.15:并发输入档多选(空=回退 ConcurrencyTier 单档)
	ConcurrencyTiers []string `json:"concurrency_tiers"`
}

type AdvParams struct {
	RequestTimeoutSec int  `json:"request_timeout_sec"`
	IgnoreEOS         bool `json:"ignore_eos"`
	// V2.2 思考模式(default=不加字段)+ 自定义请求参数
	ThinkingMode string `json:"thinking_mode"` // default/off/low/medium/high
	ExtraParams  string `json:"extra_params"`  // JSON 文本
}

type ItemsEnabled struct {
	Ladder      bool `json:"ladder"`
	Concurrency bool `json:"concurrency"`
	Cache       bool `json:"cache"`
	Stability   bool `json:"stability"`
	Quality     bool `json:"quality"`
	NIAH        bool `json:"niah"` // V2.5:NIAH 检索测试(深度×档位矩阵)
	// V2.2:能力维度筛选(空=全部);V2.4:题组/自定义选题
	CapabilityCats []string `json:"capability_cats"`
	// V2.4:选题方式 all/cats/group/custom(空=按下方字段推断,兼容旧任务)
	QualityMode        string  `json:"quality_mode"`
	QualityGroupID     int64   `json:"quality_group_id"`
	QualityQuestionIDs []int64 `json:"quality_question_ids"`
}

type TaskParams struct {
	Gen      GenParams    `json:"gen"`
	Exec     ExecParams   `json:"exec"`
	Advanced AdvParams    `json:"advanced"`
	Items    ItemsEnabled `json:"items"`
}

func DefaultTaskParams() TaskParams {
	return TaskParams{
		Gen: GenParams{Temperature: 0.7, TopP: 0.9, TopK: 20, MaxTokens: 1024},
		Exec: ExecParams{
			Warmup: 3, Repeats: 3,
			LadderTiers:          []string{"1K", "10K", "20K", "30K"},
			ConcurrencyLevels:    []int{1, 4, 8, 16, 32},
			StabilityMinutes:     1, // 默认 1 分钟:长时间压测烧 token,用户明确要求才调大
			StabilityTier:        "10K",
			StabilityConcurrency: 1,
			NIAHDepths:           []int{10, 30, 50, 70, 90},
			NIAHRepeats:          3,
			NIAHStepPct:          1,
			NIAHDepthPct:         50,
			ConcurrencyTier:      "1K",
		},
		Advanced: AdvParams{RequestTimeoutSec: 300, IgnoreEOS: false},
		Items:    ItemsEnabled{Ladder: true, Concurrency: true, Cache: true, Stability: true, Quality: true},
	}
}

func (p TaskParams) JSON() string {
	b, _ := json.Marshal(p)
	return string(b)
}

func ParseTaskParams(s string) TaskParams {
	p := DefaultTaskParams()
	if s != "" {
		_ = json.Unmarshal([]byte(s), &p)
	}
	return p
}

// ---------- 设备 ----------

type Device struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	FormFactor       string  `json:"form_factor"`
	CPUSoC           string  `json:"cpu_soc"`
	ChipModel        string  `json:"chip_model"`
	ChipCount        int     `json:"chip_count"`
	TopsInt8         float64 `json:"tops_int8"`
	TflopsFP16       float64 `json:"tflops_fp16"`
	VramGB           float64 `json:"vram_gb"`
	VramType         string  `json:"vram_type"`
	VramBwGbps       float64 `json:"vram_bw_gbps"`
	VramShared       bool    `json:"vram_shared"`
	HostPlatform     string  `json:"host_platform"`
	PcieGen          string  `json:"pcie_gen"`
	PcieLanes        string  `json:"pcie_lanes"`
	Memory           string  `json:"memory"`
	Storage          string  `json:"storage"`
	Cooling          string  `json:"cooling"`
	TdpW             float64 `json:"tdp_w"`
	Network          string  `json:"network"`
	Notes            string  `json:"notes"`
	SSHHost          string  `json:"ssh_host"`
	SSHPort          int     `json:"ssh_port"`
	SSHUser          string  `json:"ssh_user"`
	SSHAuthType      string  `json:"ssh_auth_type"`
	SSHSecretEnc     string  `json:"-"`
	SSHSecretSet     bool    `json:"ssh_secret_set"` // 界面不明文回显(F-2.2)
	MonitorAdapterID int64   `json:"monitor_adapter_id"`
	ManualPowerW     float64 `json:"manual_power_w"`
	Archived         bool    `json:"archived"`
	CreatedAt        int64   `json:"created_at"`
	UpdatedAt        int64   `json:"updated_at"`
	TaskCount        int     `json:"task_count,omitempty"`
}

const deviceCols = `id,name,form_factor,cpu_soc,chip_model,chip_count,tops_int8,tflops_fp16,
vram_gb,vram_type,vram_bw_gbps,vram_shared,host_platform,pcie_gen,pcie_lanes,memory,storage,
cooling,tdp_w,network,notes,ssh_host,ssh_port,ssh_user,ssh_auth_type,ssh_secret_enc,
monitor_adapter_id,manual_power_w,archived,created_at,updated_at`

func scanDevice(row interface{ Scan(...any) error }) (*Device, error) {
	var d Device
	var shared, archived int
	var adapterID sql.NullInt64
	err := row.Scan(&d.ID, &d.Name, &d.FormFactor, &d.CPUSoC, &d.ChipModel, &d.ChipCount,
		&d.TopsInt8, &d.TflopsFP16, &d.VramGB, &d.VramType, &d.VramBwGbps, &shared,
		&d.HostPlatform, &d.PcieGen, &d.PcieLanes, &d.Memory, &d.Storage, &d.Cooling,
		&d.TdpW, &d.Network, &d.Notes, &d.SSHHost, &d.SSHPort, &d.SSHUser, &d.SSHAuthType,
		&d.SSHSecretEnc, &adapterID, &d.ManualPowerW, &archived, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	d.VramShared = shared != 0
	d.Archived = archived != 0
	d.SSHSecretSet = d.SSHSecretEnc != ""
	if adapterID.Valid {
		d.MonitorAdapterID = adapterID.Int64
	}
	return &d, nil
}

func (s *Store) ListDevices(includeArchived bool) ([]*Device, error) {
	q := `SELECT ` + deviceCols + ` FROM devices`
	if !includeArchived {
		q += ` WHERE archived=0`
	}
	q += ` ORDER BY id DESC`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	var out []*Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	// 单连接串行:须先关闭结果集再做补充查询
	for _, d := range out {
		s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE device_id=?`, d.ID).Scan(&d.TaskCount)
	}
	return out, nil
}

func (s *Store) GetDevice(id int64) (*Device, error) {
	return scanDevice(s.db.QueryRow(`SELECT `+deviceCols+` FROM devices WHERE id=?`, id))
}

func (s *Store) CreateDevice(d *Device) error {
	d.CreatedAt, d.UpdatedAt = NowMs(), NowMs()
	res, err := s.db.Exec(`INSERT INTO devices(`+strings.Replace(deviceCols, "id,", "", 1)+`) VALUES(`+strings.Repeat("?,", 29)+`?)`,
		d.Name, d.FormFactor, d.CPUSoC, d.ChipModel, d.ChipCount, d.TopsInt8, d.TflopsFP16,
		d.VramGB, d.VramType, d.VramBwGbps, bool2int(d.VramShared), d.HostPlatform, d.PcieGen,
		d.PcieLanes, d.Memory, d.Storage, d.Cooling, d.TdpW, d.Network, d.Notes, d.SSHHost,
		d.SSHPort, d.SSHUser, d.SSHAuthType, d.SSHSecretEnc, nullableInt(d.MonitorAdapterID),
		d.ManualPowerW, bool2int(d.Archived), d.CreatedAt, d.UpdatedAt)
	if err != nil {
		return err
	}
	d.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) UpdateDevice(d *Device) error {
	d.UpdatedAt = NowMs()
	_, err := s.db.Exec(`UPDATE devices SET name=?,form_factor=?,cpu_soc=?,chip_model=?,chip_count=?,
		tops_int8=?,tflops_fp16=?,vram_gb=?,vram_type=?,vram_bw_gbps=?,vram_shared=?,host_platform=?,
		pcie_gen=?,pcie_lanes=?,memory=?,storage=?,cooling=?,tdp_w=?,network=?,notes=?,ssh_host=?,
		ssh_port=?,ssh_user=?,ssh_auth_type=?,ssh_secret_enc=?,monitor_adapter_id=?,manual_power_w=?,
		archived=?,updated_at=? WHERE id=?`,
		d.Name, d.FormFactor, d.CPUSoC, d.ChipModel, d.ChipCount, d.TopsInt8, d.TflopsFP16,
		d.VramGB, d.VramType, d.VramBwGbps, bool2int(d.VramShared), d.HostPlatform, d.PcieGen,
		d.PcieLanes, d.Memory, d.Storage, d.Cooling, d.TdpW, d.Network, d.Notes, d.SSHHost,
		d.SSHPort, d.SSHUser, d.SSHAuthType, d.SSHSecretEnc, nullableInt(d.MonitorAdapterID),
		d.ManualPowerW, bool2int(d.Archived), d.UpdatedAt, d.ID)
	return err
}

func (s *Store) DeleteDevice(id int64) error {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE device_id=?`, id).Scan(&n)
	if n > 0 {
		return fmt.Errorf("设备存在历史任务,禁止删除,可归档(F-2.4)")
	}
	_, err := s.db.Exec(`DELETE FROM devices WHERE id=?`, id)
	return err
}

func (s *Store) ArchiveDevice(id int64, archived bool) error {
	_, err := s.db.Exec(`UPDATE devices SET archived=?,updated_at=? WHERE id=?`, bool2int(archived), NowMs(), id)
	return err
}

// ---------- 语料 ----------

type CorpusVersion struct {
	ID        int64  `json:"id"`
	VersionNo string `json:"version_no"`
	Uploader  string `json:"uploader"`
	Note      string `json:"note"`
	Active    bool   `json:"active"`
	CreatedAt int64  `json:"created_at"`
}

type CorpusText struct {
	ID        int64  `json:"id"`
	VersionID int64  `json:"version_id"`
	Tier      string `json:"tier"`
	Content   string `json:"content,omitempty"`
	CharCount int    `json:"char_count"`
}

var CorpusTiers = []string{"1K", "10K", "20K", "30K"}

func (s *Store) ListCorpusVersions() ([]*CorpusVersion, error) {
	rows, err := s.db.Query(`SELECT id,version_no,uploader,note,active,created_at FROM corpus_versions ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*CorpusVersion
	for rows.Next() {
		var v CorpusVersion
		var a int
		if err := rows.Scan(&v.ID, &v.VersionNo, &v.Uploader, &v.Note, &a, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.Active = a != 0
		out = append(out, &v)
	}
	return out, rows.Err()
}

func (s *Store) GetCorpusVersion(id int64) (*CorpusVersion, error) {
	var v CorpusVersion
	var a int
	err := s.db.QueryRow(`SELECT id,version_no,uploader,note,active,created_at FROM corpus_versions WHERE id=?`, id).
		Scan(&v.ID, &v.VersionNo, &v.Uploader, &v.Note, &a, &v.CreatedAt)
	v.Active = a != 0
	return &v, err
}

func (s *Store) ActiveCorpusVersion() (*CorpusVersion, error) {
	var v CorpusVersion
	var a int
	err := s.db.QueryRow(`SELECT id,version_no,uploader,note,active,created_at FROM corpus_versions WHERE active=1 LIMIT 1`).
		Scan(&v.ID, &v.VersionNo, &v.Uploader, &v.Note, &a, &v.CreatedAt)
	v.Active = a != 0
	return &v, err
}

// LatestCorpusVersion 最新语料版本(V2.6:激活概念下线后的默认回退)
func (s *Store) LatestCorpusVersion() (*CorpusVersion, error) {
	var v CorpusVersion
	var a int
	err := s.db.QueryRow(`SELECT id,version_no,uploader,note,active,created_at FROM corpus_versions ORDER BY id DESC LIMIT 1`).
		Scan(&v.ID, &v.VersionNo, &v.Uploader, &v.Note, &a, &v.CreatedAt)
	v.Active = a != 0
	return &v, err
}

func (s *Store) NextCorpusVersionNo() string {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM corpus_versions`).Scan(&n)
	for {
		no := fmt.Sprintf("corpus-v%d", n+1)
		var c int
		s.db.QueryRow(`SELECT COUNT(*) FROM corpus_versions WHERE version_no=?`, no).Scan(&c)
		if c == 0 {
			return no
		}
		n++
	}
}

func (s *Store) CreateCorpusVersion(v *CorpusVersion) error {
	v.CreatedAt = NowMs()
	res, err := s.db.Exec(`INSERT INTO corpus_versions(version_no,uploader,note,active,created_at) VALUES(?,?,?,0,?)`,
		v.VersionNo, v.Uploader, v.Note, v.CreatedAt)
	if err != nil {
		return err
	}
	v.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) ActivateCorpusVersion(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE corpus_versions SET active=0`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE corpus_versions SET active=1 WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateCorpusVersion 修改备注/上传人(文本改动走"替换生成新版本"通道,F-3.2)
func (s *Store) UpdateCorpusVersion(id int64, uploader, note string) error {
	_, err := s.db.Exec(`UPDATE corpus_versions SET uploader=?, note=? WHERE id=?`, uploader, note, id)
	return err
}

// DeleteCorpusVersion 删除保护:被任务引用不可删(样本可溯源,F-3.5);V2.6 起激活概念下线,不再拦激活版
func (s *Store) DeleteCorpusVersion(id int64) error {
	v, err := s.GetCorpusVersion(id)
	if err != nil {
		return fmt.Errorf("版本不存在")
	}
	_ = v
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE corpus_version_id=?`, id).Scan(&n)
	if n > 0 {
		return fmt.Errorf("该版本已被 %d 个任务引用,删除会破坏样本溯源,禁止删除", n)
	}
	_, err = s.db.Exec(`DELETE FROM corpus_versions WHERE id=?`, id)
	return err
}

func (s *Store) UpsertCorpusText(versionID int64, tier, content string) error {
	cc := len([]rune(content))
	_, err := s.db.Exec(`INSERT INTO corpus_texts(version_id,tier,content,char_count) VALUES(?,?,?,?)
		ON CONFLICT(version_id,tier) DO UPDATE SET content=excluded.content, char_count=excluded.char_count`,
		versionID, tier, content, cc)
	return err
}

func (s *Store) CorpusTexts(versionID int64, withContent bool) ([]*CorpusText, error) {
	cols := "id,version_id,tier,char_count"
	if withContent {
		cols = "id,version_id,tier,content,char_count"
	}
	// 排序键支持 K/M 后缀(M=×1000K),修复 M 档与 1K 同序的缺陷
	rows, err := s.db.Query(`SELECT `+cols+` FROM corpus_texts WHERE version_id=? ORDER BY
		CAST(REPLACE(REPLACE(UPPER(tier),'K',''),'M','') AS REAL) * CASE WHEN UPPER(tier) LIKE '%M' THEN 1000 ELSE 1 END`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*CorpusText
	for rows.Next() {
		var t CorpusText
		if withContent {
			err = rows.Scan(&t.ID, &t.VersionID, &t.Tier, &t.Content, &t.CharCount)
		} else {
			err = rows.Scan(&t.ID, &t.VersionID, &t.Tier, &t.CharCount)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (s *Store) CorpusText(versionID int64, tier string) (string, error) {
	var c string
	err := s.db.QueryRow(`SELECT content FROM corpus_texts WHERE version_id=? AND tier=?`, versionID, tier).Scan(&c)
	return c, err
}

// ---------- 题库 ----------

type Question struct {
	ID       int64  `json:"id"`
	QType    string `json:"qtype"`
	Title    string `json:"title"`
	Prompt   string `json:"prompt"`
	LongText string `json:"long_text"`
	Enabled  bool   `json:"enabled"`
	Sort     int    `json:"sort"`
	// V2.2 能力题库
	Category       string `json:"category"`        // common/reasoning/instruction/code/creative/summary/needle
	ExpectedAnswer string `json:"expected_answer"` // 参考答案/评分要点
	JudgeMode      string `json:"judge_mode"`      // rule/llm/manual
	ArtifactType   string `json:"artifact_type"`   // none/html/code
	UseGoal        bool   `json:"use_goal"`        // V2.11:目标驱动(goal 契约,产物题默认开)
}

func (s *Store) ListQuestions(enabledOnly bool) ([]*Question, error) {
	q := `SELECT id,qtype,title,prompt,long_text,enabled,sort,
		COALESCE(category,''),COALESCE(expected_answer,''),COALESCE(judge_mode,'llm'),COALESCE(artifact_type,'none'),COALESCE(use_goal,0) FROM questions`
	if enabledOnly {
		q += ` WHERE enabled=1`
	}
	q += ` ORDER BY id` // 按 ID 升序(修改清单-4)
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Question
	for rows.Next() {
		var q Question
		var e int
		var ug int
		if err := rows.Scan(&q.ID, &q.QType, &q.Title, &q.Prompt, &q.LongText, &e, &q.Sort,
			&q.Category, &q.ExpectedAnswer, &q.JudgeMode, &q.ArtifactType, &ug); err != nil {
			return nil, err
		}
		q.Enabled = e != 0
		q.UseGoal = ug != 0
		out = append(out, &q)
	}
	return out, rows.Err()
}

func (s *Store) CreateQuestion(q *Question) error {
	res, err := s.db.Exec(`INSERT INTO questions(qtype,title,prompt,long_text,enabled,sort,category,expected_answer,judge_mode,artifact_type,use_goal)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		q.QType, q.Title, q.Prompt, q.LongText, bool2int(q.Enabled), q.Sort,
		q.Category, q.ExpectedAnswer, q.JudgeMode, q.ArtifactType, bool2int(q.UseGoal))
	if err != nil {
		return err
	}
	q.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) UpdateQuestion(q *Question) error {
	_, err := s.db.Exec(`UPDATE questions SET qtype=?,title=?,prompt=?,long_text=?,enabled=?,sort=?,
		category=?,expected_answer=?,judge_mode=?,artifact_type=?,use_goal=? WHERE id=?`,
		q.QType, q.Title, q.Prompt, q.LongText, bool2int(q.Enabled), q.Sort,
		q.Category, q.ExpectedAnswer, q.JudgeMode, q.ArtifactType, bool2int(q.UseGoal), q.ID)
	return err
}

func (s *Store) DeleteQuestion(id int64) error {
	_, err := s.db.Exec(`DELETE FROM questions WHERE id=?`, id)
	return err
}

// ---------- 题组(V2.4:自定义编组,全局共享) ----------

type QuestionGroup struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Note        string  `json:"note"`
	CreatedAt   int64   `json:"created_at"`
	QuestionIDs []int64 `json:"question_ids"`
}

func (s *Store) ListQuestionGroups() ([]*QuestionGroup, error) {
	rows, err := s.db.Query(`SELECT id,name,note,created_at FROM question_groups ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var out []*QuestionGroup
	for rows.Next() {
		var g QuestionGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Note, &g.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, &g)
	}
	rows.Close()
	// 单连接:成员查询须在结果集关闭后进行
	for _, g := range out {
		g.QuestionIDs = s.groupQuestionIDs(g.ID)
	}
	return out, nil
}

func (s *Store) groupQuestionIDs(groupID int64) []int64 {
	rows, err := s.db.Query(`SELECT question_id FROM question_group_items WHERE group_id=? ORDER BY question_id`, groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

// GroupQuestionIDs 供 runner 按组取题
func (s *Store) GroupQuestionIDs(groupID int64) []int64 { return s.groupQuestionIDs(groupID) }

// setGroupQuestions 全量替换组成员(先删后插)
func (s *Store) setGroupQuestions(groupID int64, ids []int64) error {
	if _, err := s.db.Exec(`DELETE FROM question_group_items WHERE group_id=?`, groupID); err != nil {
		return err
	}
	for _, qid := range ids {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO question_group_items(group_id,question_id) VALUES(?,?)`, groupID, qid); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateQuestionGroup(g *QuestionGroup) error {
	g.CreatedAt = NowMs()
	res, err := s.db.Exec(`INSERT INTO question_groups(name,note,created_at) VALUES(?,?,?)`, g.Name, g.Note, g.CreatedAt)
	if err != nil {
		return err
	}
	g.ID, _ = res.LastInsertId()
	return s.setGroupQuestions(g.ID, g.QuestionIDs)
}

func (s *Store) UpdateQuestionGroup(g *QuestionGroup) error {
	if _, err := s.db.Exec(`UPDATE question_groups SET name=?,note=? WHERE id=?`, g.Name, g.Note, g.ID); err != nil {
		return err
	}
	return s.setGroupQuestions(g.ID, g.QuestionIDs)
}

func (s *Store) DeleteQuestionGroup(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM question_group_items WHERE group_id=?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM question_groups WHERE id=?`, id)
	return err
}

// ---------- 项目 ----------

type Project struct {
	ID         int64  `json:"id"`
	Type       string `json:"type"` // hardware/model(V2.2)
	DeviceID   int64  `json:"device_id"`
	DeviceName string `json:"device_name,omitempty"`
	HwJSON     string `json:"hw_json"` // 本地硬件信息快照(V2.2)
	Name       string `json:"name"`
	Tester     string `json:"tester"`
	Note       string `json:"note"`
	Hidden     bool   `json:"hidden"` // V3.0.1:隐藏(不删除),列表/对比/首页不可见
	CreatedAt  int64  `json:"created_at"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
	// 派生状态(F-4.3)
	Status    string `json:"status"`
	TaskTotal int    `json:"task_total"`
	TaskDone  int    `json:"task_done"`
	// V2.4:项目即任务——直跳任务详情 + 测试项标签 + 模型名
	TaskID    int64    `json:"task_id"`
	ModelName string   `json:"model_name"`
	Tags      []string `json:"tags"`
	Progress  int      `json:"progress"` // V2.6:任务实时进度(与任务详情/首页同口径)
}

func (s *Store) fillProjectDerived(p *Project) {
	rows, _ := s.db.Query(`SELECT status, COUNT(*) FROM tasks WHERE project_id=? GROUP BY status`, p.ID)
	states := map[string]int{}
	if rows != nil {
		for rows.Next() {
			var st string
			var c int
			rows.Scan(&st, &c)
			states[st] = c
			p.TaskTotal += c
			if st == "done" || st == "failed" || st == "cancelled" || st == "interrupted" {
				p.TaskDone += c
			}
		}
		rows.Close()
	}
	switch {
	case p.TaskTotal == 0:
		p.Status = "pending"
	case states["running"] > 0:
		p.Status = "running"
	case states["paused"] > 0:
		p.Status = "paused" // V2.19:有暂停任务 → 项目显示「已暂停」,可断点续跑
	case states["queued"] > 0:
		p.Status = "queued" // V2.16:无运行仅有排队 → 排队中(与首页任务级口径一致)
	case p.TaskDone == p.TaskTotal:
		if states["failed"]+states["interrupted"]+states["cancelled"] > 0 && states["done"] > 0 {
			p.Status = "partial_failed"
		} else if states["done"] == p.TaskTotal {
			p.Status = "done"
		} else {
			p.Status = "partial_failed"
		}
	case p.TaskDone == 0:
		p.Status = "pending" // 全部任务待执行(修改清单-3)
	default:
		p.Status = "running"
	}
	// 项目即任务:带出首个(唯一)任务的跳转锚点/测试项标签/实时进度
	var paramsJSON string
	err := s.db.QueryRow(`SELECT id, COALESCE(model_name,''), COALESCE(params_json,''), progress FROM tasks WHERE project_id=? ORDER BY id LIMIT 1`, p.ID).
		Scan(&p.TaskID, &p.ModelName, &paramsJSON, &p.Progress)
	if err == nil && paramsJSON != "" {
		items := ParseTaskParams(paramsJSON).Items
		tags := []string{}
		if items.Quality {
			tags = append(tags, "能力")
		}
		if items.Ladder {
			tags = append(tags, "阶梯")
		}
		if items.Concurrency {
			tags = append(tags, "并发")
		}
		if items.Cache {
			tags = append(tags, "缓存")
		}
		if items.Stability {
			tags = append(tags, "稳定性")
		}
		if items.NIAH {
			tags = append(tags, "NIAH")
		}
		p.Tags = tags
	}
}

// ProjectFilter 项目列表筛选(名称/设备/测试人员/状态/时间范围,§修改清单-1)
type ProjectFilter struct {
	DeviceID int64
	Tester   string
	Name     string
	Hidden   bool // true=只查隐藏项目;false=只查未隐藏(默认)
	Status   string // pending/running/done/partial_failed(派生,内存过滤)
	FromMs   int64
	ToMs     int64
	Progress string // notstarted/running/alldone(派生,内存过滤)
}

func (s *Store) ListProjects(f ProjectFilter) ([]*Project, error) {
	q := `SELECT p.id,p.type,COALESCE(p.device_id,0),COALESCE(d.name,''),COALESCE(p.hw_json,''),p.name,p.tester,p.note,COALESCE(p.hidden,0),p.created_at,COALESCE(p.started_at,0),COALESCE(p.finished_at,0)
		FROM projects p LEFT JOIN devices d ON d.id=p.device_id WHERE COALESCE(p.hidden,0)=?`
	var args []any
	hiddenInt := 0
	if f.Hidden {
		hiddenInt = 1
	}
	args = append(args, hiddenInt)
	if f.DeviceID > 0 {
		q += ` AND p.device_id=?`
		args = append(args, f.DeviceID)
	}
	if f.Tester != "" {
		q += ` AND p.tester LIKE ?`
		args = append(args, "%"+f.Tester+"%")
	}
	if f.Name != "" {
		q += ` AND p.name LIKE ?`
		args = append(args, "%"+f.Name+"%")
	}
	if f.FromMs > 0 {
		q += ` AND p.created_at>=?`
		args = append(args, f.FromMs)
	}
	if f.ToMs > 0 {
		q += ` AND p.created_at<=?`
		args = append(args, f.ToMs)
	}
	q += ` ORDER BY p.id DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Type, &p.DeviceID, &p.DeviceName, &p.HwJSON, &p.Name, &p.Tester, &p.Note, &p.Hidden, &p.CreatedAt, &p.StartedAt, &p.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	// 派生状态补充查询必须在结果集关闭后进行(单连接)
	for _, p := range out {
		s.fillProjectDerived(p)
	}
	// 派生字段内存过滤(状态/进度桶)
	if f.Status == "" && f.Progress == "" {
		return out, nil
	}
	var filtered []*Project
	for _, p := range out {
		if f.Status != "" && p.Status != f.Status {
			continue
		}
		switch f.Progress {
		case "notstarted":
			if !(p.TaskTotal == 0 || p.TaskDone == 0) {
				continue
			}
		case "running":
			if !(p.TaskTotal > 0 && p.TaskDone > 0 && p.TaskDone < p.TaskTotal) {
				continue
			}
		case "alldone":
			if !(p.TaskTotal > 0 && p.TaskDone == p.TaskTotal) {
				continue
			}
		}
		filtered = append(filtered, p)
	}
	return filtered, nil
}

func (s *Store) GetProject(id int64) (*Project, error) {
	var p Project
	err := s.db.QueryRow(`SELECT p.id,p.type,COALESCE(p.device_id,0),COALESCE(d.name,''),COALESCE(p.hw_json,''),p.name,p.tester,p.note,COALESCE(p.hidden,0),p.created_at,COALESCE(p.started_at,0),COALESCE(p.finished_at,0)
		FROM projects p LEFT JOIN devices d ON d.id=p.device_id WHERE p.id=?`, id).
		Scan(&p.ID, &p.Type, &p.DeviceID, &p.DeviceName, &p.HwJSON, &p.Name, &p.Tester, &p.Note, &p.Hidden, &p.CreatedAt, &p.StartedAt, &p.FinishedAt)
	if err != nil {
		return nil, err
	}
	s.fillProjectDerived(&p)
	return &p, nil
}

func (s *Store) CreateProject(p *Project) error {
	p.CreatedAt = NowMs()
	if p.Type == "" {
		p.Type = "hardware"
	}
	res, err := s.db.Exec(`INSERT INTO projects(type,device_id,hw_json,name,tester,note,created_at) VALUES(?,?,?,?,?,?,?)`,
		p.Type, nullableInt(p.DeviceID), p.HwJSON, p.Name, p.Tester, p.Note, p.CreatedAt)
	if err != nil {
		return err
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) UpdateProject(p *Project) error {
	_, err := s.db.Exec(`UPDATE projects SET name=?,tester=?,note=?,hw_json=? WHERE id=?`, p.Name, p.Tester, p.Note, p.HwJSON, p.ID)
	return err
}

// SetProjectHidden 隐藏/恢复项目(V3.0.1):隐藏不删数据,列表/对比/首页不可见
func (s *Store) SetProjectHidden(id int64, hidden bool) error {
	v := 0
	if hidden {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE projects SET hidden=? WHERE id=?`, v, id)
	return err
}

func (s *Store) DeleteProject(id int64) error {
	_, err := s.db.Exec(`DELETE FROM projects WHERE id=?`, id)
	return err
}

func (s *Store) TouchProjectStarted(id int64) {
	s.db.Exec(`UPDATE projects SET started_at=COALESCE(started_at,?) WHERE id=?`, NowMs(), id)
}

func (s *Store) TouchProjectFinished(id int64) {
	s.db.Exec(`UPDATE projects SET finished_at=? WHERE id=?`, NowMs(), id)
}

// ---------- 任务 ----------

type Task struct {
	ID              int64  `json:"id"`
	ProjectID       int64  `json:"project_id"`
	DeviceID        int64  `json:"device_id"`
	DeviceName      string `json:"device_name,omitempty"`
	Seq             int    `json:"seq"`
	APIBaseURL      string `json:"api_base_url"`
	ModelName       string `json:"model_name"`
	ModelHFID       string `json:"model_hf_id"`
	ModelMSID       string `json:"model_ms_id"`
	Quant           string `json:"quant"`
	ContextLen      int    `json:"context_len"`
	CacheEnabled    string `json:"cache_enabled"`
	CacheNote       string `json:"cache_note"`
	APIKeyEnc       string `json:"-"`                 // 加密存储,不明文回显
	APIKeySet       bool   `json:"api_key_set"`
	ParamsJSON      string `json:"params_json"`
	CorpusVersionID int64  `json:"corpus_version_id"`
	CorpusVersionNo string `json:"corpus_version_no,omitempty"`
	Status          string `json:"status"`
	Progress        int    `json:"progress"`
	CurrentItem     string `json:"current_item"`
	Error           string `json:"error"`
	RunNo           int    `json:"run_no"`
	CreatedAt       int64  `json:"created_at"`
	StartedAt       int64  `json:"started_at"`
	FinishedAt      int64  `json:"finished_at"`
}

const taskCols = `t.id,t.project_id,COALESCE(t.device_id,0),COALESCE(d.name,''),t.seq,t.api_base_url,t.model_name,
t.model_hf_id,t.model_ms_id,t.quant,t.context_len,t.cache_enabled,t.cache_note,t.api_key_enc,t.params_json,
COALESCE(t.corpus_version_id,0),COALESCE(cv.version_no,''),t.status,t.progress,t.current_item,
t.error,t.run_no,t.created_at,COALESCE(t.started_at,0),COALESCE(t.finished_at,0)`

const taskJoin = ` FROM tasks t LEFT JOIN devices d ON d.id=t.device_id
LEFT JOIN corpus_versions cv ON cv.id=t.corpus_version_id`

func scanTask(row interface{ Scan(...any) error }) (*Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.ProjectID, &t.DeviceID, &t.DeviceName, &t.Seq, &t.APIBaseURL,
		&t.ModelName, &t.ModelHFID, &t.ModelMSID, &t.Quant, &t.ContextLen, &t.CacheEnabled,
		&t.CacheNote, &t.APIKeyEnc, &t.ParamsJSON, &t.CorpusVersionID, &t.CorpusVersionNo, &t.Status,
		&t.Progress, &t.CurrentItem, &t.Error, &t.RunNo, &t.CreatedAt, &t.StartedAt, &t.FinishedAt)
	t.APIKeySet = t.APIKeyEnc != ""
	return &t, err
}

func (s *Store) GetTask(id int64) (*Task, error) {
	return scanTask(s.db.QueryRow(`SELECT `+taskCols+taskJoin+` WHERE t.id=?`, id))
}

func (s *Store) ListTasks(projectID, deviceID int64, status string) ([]*Task, error) {
	q := `SELECT ` + taskCols + taskJoin + ` WHERE 1=1`
	var args []any
	if projectID > 0 {
		q += ` AND t.project_id=?`
		args = append(args, projectID)
	}
	if deviceID > 0 {
		q += ` AND t.device_id=?`
		args = append(args, deviceID)
	}
	if status != "" {
		q += ` AND t.status=?`
		args = append(args, status)
	}
	q += ` ORDER BY t.project_id DESC, t.seq, t.id`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) CreateTask(t *Task) error {
	t.CreatedAt = NowMs()
	t.Status = "pending"
	t.RunNo = 1
	var maxSeq int
	s.db.QueryRow(`SELECT COALESCE(MAX(seq),0) FROM tasks WHERE project_id=?`, t.ProjectID).Scan(&maxSeq)
	t.Seq = maxSeq + 1
	res, err := s.db.Exec(`INSERT INTO tasks(project_id,device_id,seq,api_base_url,model_name,model_hf_id,
		model_ms_id,quant,context_len,cache_enabled,cache_note,api_key_enc,params_json,corpus_version_id,status,
		progress,current_item,error,run_no,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ProjectID, nullableInt(t.DeviceID), t.Seq, t.APIBaseURL, t.ModelName, t.ModelHFID, t.ModelMSID, t.Quant,
		t.ContextLen, t.CacheEnabled, t.CacheNote, t.APIKeyEnc, t.ParamsJSON, nullableInt(t.CorpusVersionID),
		t.Status, 0, "", "", t.RunNo, t.CreatedAt)
	if err != nil {
		return err
	}
	t.ID, _ = res.LastInsertId()
	return nil
}

// UpdateTaskConfig 编辑任务配置:仅 待执行/失败/已取消/中断 可改(A3 口径);
// 从终态编辑后重置为待执行且 run_no+1(历史 run 保留)
func (s *Store) UpdateTaskConfig(t *Task) error {
	res, err := s.db.Exec(`UPDATE tasks SET api_base_url=?,model_name=?,model_hf_id=?,model_ms_id=?,quant=?,
		context_len=?,cache_enabled=?,cache_note=?,api_key_enc=?,params_json=?,corpus_version_id=?
		WHERE id=? AND status IN ('pending','failed','cancelled','interrupted','paused')`,
		t.APIBaseURL, t.ModelName, t.ModelHFID, t.ModelMSID, t.Quant, t.ContextLen, t.CacheEnabled,
		t.CacheNote, t.APIKeyEnc, t.ParamsJSON, nullableInt(t.CorpusVersionID), t.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("当前状态不可编辑(执行中/排队中请先取消;已完成任务请复制为新任务)")
	}
	// 终态(失败/取消/中断)保存后重置为待执行、run+1;暂停态保持暂停,点「继续跑」按新配置断点续跑
	s.db.Exec(`UPDATE tasks SET status='pending', progress=0, current_item='', error='', run_no=run_no+1
		WHERE id=? AND status IN ('failed','cancelled','interrupted')`, t.ID)
	return nil
}

func (s *Store) DeleteTask(id int64) error {
	var st string
	if err := s.db.QueryRow(`SELECT status FROM tasks WHERE id=?`, id).Scan(&st); err != nil {
		return err
	}
	if st != "pending" && st != "interrupted" && st != "cancelled" && st != "failed" && st != "done" && st != "paused" {
		return fmt.Errorf("执行中的任务不可删除")
	}
	_, err := s.db.Exec(`DELETE FROM tasks WHERE id=?`, id)
	return err
}

func (s *Store) SetTaskStatus(id int64, status string, errMsg string) {
	s.db.Exec(`UPDATE tasks SET status=?, error=? WHERE id=?`, status, errMsg, id)
}

// ClaimTask 原子认领排队任务(queued→running),并发取消时认领失败
func (s *Store) ClaimTask(id int64) bool {
	res, err := s.db.Exec(`UPDATE tasks SET status='running', started_at=? WHERE id=? AND status='queued'`, NowMs(), id)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

func (s *Store) SetTaskProgress(id int64, progress int, currentItem string) {
	s.db.Exec(`UPDATE tasks SET progress=?, current_item=? WHERE id=?`, progress, currentItem, id)
}

func (s *Store) TaskStarted(id int64) {
	s.db.Exec(`UPDATE tasks SET status='running', started_at=?, error='' WHERE id=?`, NowMs(), id)
}

func (s *Store) TaskFinished(id int64, status, errMsg string) {
	s.db.Exec(`UPDATE tasks SET status=?, error=?, finished_at=?, progress=CASE WHEN ?='done' THEN 100 ELSE progress END, current_item='' WHERE id=?`,
		status, errMsg, NowMs(), status, id)
}

// FreezeTaskSnapshots 任务终态时冻结语料+题目快照(V2.12):
// 数据包/追溯从此读快照,之后题库/语料库的改动或删除不影响历史任务。
// 幂等:先清后插。题目以实际测过的(quality_answers)为准,题库已删的题也保留标题。
func (s *Store) FreezeTaskSnapshots(taskID int64, runNo int) {
	t, err := s.GetTask(taskID)
	if err != nil {
		return
	}
	// 事务化:崩溃在半路上不会留下"删了没插"的空快照
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	tx.Exec(`DELETE FROM task_corpus_snapshot WHERE task_id=? AND run_no=?`, taskID, runNo)
	tx.Exec(`DELETE FROM task_question_snapshot WHERE task_id=? AND run_no=?`, taskID, runNo)
	if t.CorpusVersionID > 0 {
		tx.Exec(`INSERT INTO task_corpus_snapshot(task_id,run_no,version_no,tier,content,char_count)
			SELECT ?,?,?,tier,content,char_count FROM corpus_texts WHERE version_id=?`,
			taskID, runNo, t.CorpusVersionNo, t.CorpusVersionID)
	}
	tx.Exec(`INSERT INTO task_question_snapshot(task_id,run_no,question_id,title,prompt,long_text,qtype,category,expected_answer,judge_mode,artifact_type,use_goal)
		SELECT ?,?,qa.question_id,COALESCE(q.title,qa.question_title),COALESCE(q.prompt,''),COALESCE(q.long_text,''),
			COALESCE(q.qtype,''),COALESCE(q.category,''),COALESCE(q.expected_answer,''),COALESCE(q.judge_mode,''),
			COALESCE(q.artifact_type,''),COALESCE(q.use_goal,0)
		FROM (SELECT DISTINCT question_id, question_title FROM quality_answers WHERE task_id=? AND run_no=?) qa
		LEFT JOIN questions q ON q.id=qa.question_id`,
		taskID, runNo, taskID, runNo)
	tx.Commit()
}

// ---------- 测试项 ----------

type TestItem struct {
	ID         int64  `json:"id"`
	TaskID     int64  `json:"task_id"`
	RunNo      int    `json:"run_no"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Error      string `json:"error"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
}

// ListTestItems 默认展示最新 run 的测试项(F-5.4);runNo<=0 时取任务当前 run_no
func (s *Store) ListTestItems(taskID int64, runNo int) ([]*TestItem, error) {
	if runNo <= 0 {
		s.db.QueryRow(`SELECT run_no FROM tasks WHERE id=?`, taskID).Scan(&runNo)
	}
	rows, err := s.db.Query(`SELECT id,task_id,run_no,type,status,error,COALESCE(started_at,0),COALESCE(finished_at,0)
		FROM test_items WHERE task_id=? AND run_no=? ORDER BY CASE type WHEN 'ladder' THEN 1 WHEN 'concurrency' THEN 2 WHEN 'cache' THEN 3 WHEN 'stability' THEN 4 WHEN 'quality' THEN 5 ELSE 6 END`, taskID, runNo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*TestItem
	for rows.Next() {
		var it TestItem
		if err := rows.Scan(&it.ID, &it.TaskID, &it.RunNo, &it.Type, &it.Status, &it.Error, &it.StartedAt, &it.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, &it)
	}
	return out, rows.Err()
}

func (s *Store) GetTestItem(id int64) (*TestItem, error) {
	var it TestItem
	err := s.db.QueryRow(`SELECT id,task_id,run_no,type,status,error,COALESCE(started_at,0),COALESCE(finished_at,0) FROM test_items WHERE id=?`, id).
		Scan(&it.ID, &it.TaskID, &it.RunNo, &it.Type, &it.Status, &it.Error, &it.StartedAt, &it.FinishedAt)
	return &it, err
}

// ---------- V2.20 断点续跑支持 ----------

// FindInterruptedItem 找该任务/类型/run 的中断项行(暂停中断续跑时复用,样本 item_id 不断链)
func (s *Store) FindInterruptedItem(taskID int64, runNo int, typ string) int64 {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM test_items WHERE task_id=? AND run_no=? AND type=? AND status='interrupted' ORDER BY id DESC LIMIT 1`,
		taskID, runNo, typ).Scan(&id)
	if err != nil {
		return 0
	}
	return id
}

// DoneSampleCounts 项内已有有效样本统计(断点续跑):key = "phase|tier" 或 "phase|tier|concurrency"
func (s *Store) DoneSampleCounts(itemID int64) map[string]int {
	out := map[string]int{}
	rows, err := s.db.Query(`SELECT phase, tier, concurrency, COUNT(*) FROM request_samples
		WHERE item_id=? AND status='ok' GROUP BY phase, tier, concurrency`, itemID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var phase, tier string
		var conc, n int
		rows.Scan(&phase, &tier, &conc, &n)
		out[fmt.Sprintf("%s|%s", phase, tier)] += n
		out[fmt.Sprintf("%s|%s|%d", phase, tier, conc)] += n
	}
	return out
}

// SampleTimeRange 项内样本首末时间(ms)(稳定性续跑估算已跑时长)
func (s *Store) SampleTimeRange(itemID int64) (first, last int64) {
	s.db.QueryRow(`SELECT COALESCE(MIN(send_ts),0), COALESCE(MAX(send_ts),0) FROM request_samples WHERE item_id=? AND status='ok'`,
		itemID).Scan(&first, &last)
	return
}

func (s *Store) CreateTestItem(taskID int64, typ string, runNo int) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO test_items(task_id,run_no,type,status) VALUES(?,?,?,'pending')`, taskID, runNo, typ)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ItemStarted(id int64) {
	s.db.Exec(`UPDATE test_items SET status='running', started_at=?, error='' WHERE id=?`, NowMs(), id)
}

func (s *Store) ItemFinished(id int64, status, errMsg string) {
	s.db.Exec(`UPDATE test_items SET status=?, error=?, finished_at=? WHERE id=?`, status, errMsg, NowMs(), id)
}

// ---------- 样本/日志写入 ----------

type Sample struct {
	TaskID           int64
	ItemID           int64
	RunNo            int
	Phase            string
	Tier             string
	Concurrency      int
	Seq              int
	SendTS           int64
	FirstChunkTS     int64
	EndTS            int64
	TtftMs           float64
	TotalMs          float64
	PromptTokens     int
	CompletionTokens int
	TokensEstimated  bool
	OutputIncomplete bool
	LengthMismatch   bool
	Speed            float64
	Status           string
	Error            string
	RawResponse      string
	RequestID        int64 // 请求体去重引用(request_bodies)
	CachedTokens     int   // 服务返回的缓存命中 token(usage.prompt_tokens_details.cached_tokens)
	UsageSuspect     bool // usage 明显异常(超 max_tokens),不参与中位数
	ReasoningOnly    bool // 全思维链无正文
	Meta             string // V2.5:模块自定义数据(JSON,如 NIAH 的针与命中)
}

func (s *Store) InsertSample(sm *Sample) (int64, error) {
	raw := sm.RawResponse
	// 全量保存返回内容(V2.2 全量日志要求)
	res, err := s.db.Exec(`INSERT INTO request_samples(task_id,item_id,run_no,phase,tier,concurrency,seq,
		send_ts,first_chunk_ts,end_ts,ttft_ms,total_ms,prompt_tokens,completion_tokens,tokens_estimated,
		output_incomplete,length_mismatch,speed,status,error,raw_response,usage_suspect,reasoning_only,request_id,cached_tokens,meta)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sm.TaskID, nullableInt(sm.ItemID), sm.RunNo, sm.Phase, sm.Tier, sm.Concurrency, sm.Seq,
		sm.SendTS, sm.FirstChunkTS, sm.EndTS, sm.TtftMs, sm.TotalMs, sm.PromptTokens, sm.CompletionTokens,
		bool2int(sm.TokensEstimated), bool2int(sm.OutputIncomplete), bool2int(sm.LengthMismatch),
		sm.Speed, sm.Status, sm.Error, raw, bool2int(sm.UsageSuspect), bool2int(sm.ReasoningOnly),
		nullableInt(sm.RequestID), sm.CachedTokens, sm.Meta)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SaveRequestBody 请求体去重存储(相同 body 只存一份)
func (s *Store) SaveRequestBody(body string) int64 {
	if body == "" {
		return 0
	}
	sum := sha256.Sum256([]byte(body))
	hash := hex.EncodeToString(sum[:16])
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM request_bodies WHERE hash=?`, hash).Scan(&id); err == nil {
		return id
	}
	res, err := s.db.Exec(`INSERT INTO request_bodies(hash,body) VALUES(?,?)`, hash, body)
	if err != nil {
		return 0
	}
	id, _ = res.LastInsertId()
	return id
}

func (s *Store) GetRequestBody(id int64) string {
	var b string
	s.db.QueryRow(`SELECT body FROM request_bodies WHERE id=?`, id).Scan(&b)
	return b
}

// QualityAnswer V2.2:带评判与产物字段
type QualityAnswer struct {
	TaskID       int64
	RunNo        int
	QuestionID   int64
	Title        string
	Answer       string
	AutoFlag     string
	AutoReason   string
	JudgeScore   string // pass/partial/fail
	JudgeReason  string
	ArtifactPath string
	ArtifactOK   bool
	ArtifactType string // V2.11:运行时快照,不随题库后续修改变化(任务68事故)
}

func (s *Store) InsertQualityAnswer(qa *QualityAnswer) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO quality_answers(task_id,run_no,question_id,question_title,answer_text,
		auto_flag,auto_reason,judge_score,judge_reason,artifact_path,artifact_ok,artifact_type)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		qa.TaskID, qa.RunNo, qa.QuestionID, qa.Title, qa.Answer,
		qa.AutoFlag, qa.AutoReason, qa.JudgeScore, qa.JudgeReason, qa.ArtifactPath, bool2int(qa.ArtifactOK), qa.ArtifactType)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) MarkQualityAnswer(id int64, flag, note string) error {
	_, err := s.db.Exec(`UPDATE quality_answers SET manual_flag=?, manual_note=? WHERE id=?`, flag, note, id)
	return err
}

func (s *Store) AddTaskLog(taskID int64, level, message string) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO task_logs(task_id,ts,level,message) VALUES(?,?,?,?)`, taskID, NowMs(), level, message)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ---------- 辅助 ----------

func bool2int(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableInt(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

// ---------- 设备规格书附件(入库 BLOB) ----------

type DeviceFile struct {
	ID          int64  `json:"id"`
	DeviceID    int64  `json:"device_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	CreatedAt   int64  `json:"created_at"`
}

func (s *Store) AddDeviceFile(deviceID int64, filename, contentType string, data []byte) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO device_files(device_id,filename,content_type,size,data,created_at)
		VALUES(?,?,?,?,?,?)`, deviceID, filename, contentType, len(data), data, NowMs())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListDeviceFiles(deviceID int64) ([]*DeviceFile, error) {
	rows, err := s.db.Query(`SELECT id,device_id,filename,content_type,size,created_at
		FROM device_files WHERE device_id=? ORDER BY id DESC`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*DeviceFile{}
	for rows.Next() {
		var f DeviceFile
		if err := rows.Scan(&f.ID, &f.DeviceID, &f.Filename, &f.ContentType, &f.Size, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &f)
	}
	return out, rows.Err()
}

func (s *Store) GetDeviceFile(id int64) (*DeviceFile, []byte, error) {
	var f DeviceFile
	var data []byte
	err := s.db.QueryRow(`SELECT id,device_id,filename,content_type,size,created_at,data FROM device_files WHERE id=?`, id).
		Scan(&f.ID, &f.DeviceID, &f.Filename, &f.ContentType, &f.Size, &f.CreatedAt, &data)
	if err != nil {
		return nil, nil, err
	}
	return &f, data, nil
}

func (s *Store) DeleteDeviceFile(id int64) error {
	_, err := s.db.Exec(`DELETE FROM device_files WHERE id=?`, id)
	return err
}

// ---------- 项目规格书附件(V2.2,规格书改挂项目) ----------

type ProjectFile struct {
	ID          int64  `json:"id"`
	ProjectID   int64  `json:"project_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	CreatedAt   int64  `json:"created_at"`
}

func (s *Store) AddProjectFile(projectID int64, filename, contentType string, data []byte) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO project_files(project_id,filename,content_type,size,data,created_at)
		VALUES(?,?,?,?,?,?)`, projectID, filename, contentType, len(data), data, NowMs())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListProjectFiles(projectID int64) ([]*ProjectFile, error) {
	rows, err := s.db.Query(`SELECT id,project_id,filename,content_type,size,created_at
		FROM project_files WHERE project_id=? ORDER BY id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ProjectFile{}
	for rows.Next() {
		var f ProjectFile
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Filename, &f.ContentType, &f.Size, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &f)
	}
	return out, rows.Err()
}

func (s *Store) GetProjectFile(id int64) (*ProjectFile, []byte, error) {
	var f ProjectFile
	var data []byte
	err := s.db.QueryRow(`SELECT id,project_id,filename,content_type,size,created_at,data FROM project_files WHERE id=?`, id).
		Scan(&f.ID, &f.ProjectID, &f.Filename, &f.ContentType, &f.Size, &f.CreatedAt, &data)
	if err != nil {
		return nil, nil, err
	}
	return &f, data, nil
}

func (s *Store) DeleteProjectFile(id int64) error {
	_, err := s.db.Exec(`DELETE FROM project_files WHERE id=?`, id)
	return err
}
