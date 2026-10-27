// 出厂自带种子(V2.24):语料 corpus-public-v1(公开仿真语料,gzip 内嵌)+ 质量题库(29 题)
// 仅全新安装(对应表为空)时载入;已部署环境不受影响
package store

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"io"
)

//go:embed seed/corpus.json.gz
var seedCorpusGz []byte

//go:embed seed/questions.json
var seedQuestionsJSON []byte

type seedQuestion struct {
	QType          string `json:"qtype"`
	Title          string `json:"title"`
	Prompt         string `json:"prompt"`
	LongText       string `json:"long_text"`
	Category       string `json:"category"`
	ExpectedAnswer string `json:"expected_answer"`
	JudgeMode      string `json:"judge_mode"`
	ArtifactType   string `json:"artifact_type"`
	UseGoal        int    `json:"use_goal"`
	Sort           int    `json:"sort"`
}

// seedEmbeddedQuestions 载入出厂题库;成功返回 true
func (s *Store) seedEmbeddedQuestions() bool {
	// 一次性:做过就不再重复(用户后续删题不会被复活)
	if v, ok := s.GetSetting("factory_seed_v224_done"); ok && v == "1" {
		return true
	}
	var qs []seedQuestion
	if err := json.Unmarshal(seedQuestionsJSON, &qs); err != nil || len(qs) == 0 {
		return false
	}
	for _, q := range qs {
		// 按标题幂等(迁移 SQL 的文档 4 题等已存在的跳过)
		s.db.Exec(`INSERT INTO questions(qtype,title,prompt,long_text,enabled,sort,category,expected_answer,judge_mode,artifact_type,use_goal)
			SELECT ?,?,?,?,1,?,?,?,?,?,? WHERE NOT EXISTS(SELECT 1 FROM questions WHERE title=?)`,
			q.QType, q.Title, q.Prompt, q.LongText, q.Sort, q.Category, q.ExpectedAnswer, q.JudgeMode, q.ArtifactType, q.UseGoal, q.Title)
	}
	s.SetSetting("factory_seed_v224_done", "1")
	return true
}

// seedEmbeddedCorpus 载入出厂语料(corpus-public-v1,gzip 解压)
func (s *Store) seedEmbeddedCorpus() bool {
	zr, err := gzip.NewReader(bytes.NewReader(seedCorpusGz))
	if err != nil {
		return false
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return false
	}
	var sc struct {
		VersionNo string            `json:"version_no"`
		Note      string            `json:"note"`
		Texts     map[string]string `json:"texts"`
	}
	if err := json.Unmarshal(raw, &sc); err != nil || len(sc.Texts) == 0 {
		return false
	}
	if sc.VersionNo == "" {
		sc.VersionNo = "corpus-public-v1"
	}
	v := &CorpusVersion{VersionNo: sc.VersionNo, Uploader: "factory", Note: sc.Note}
	if err := s.CreateCorpusVersion(v); err != nil {
		return false
	}
	for tier, content := range sc.Texts {
		s.UpsertCorpusText(v.ID, tier, content)
	}
	return true
}


