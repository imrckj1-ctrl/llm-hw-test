// Package store SQLite 访问层。全部写操作单连接串行(§7.2)。
package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// NowMs 当前 Unix 毫秒
func NowMs() int64 { return time.Now().UnixMilli() }

type Store struct {
	db        *sql.DB
	secretKey []byte // AES-GCM 密钥,用于 SSH 凭据加密
}

func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// 单连接串行化所有读写(文档 §7.2:全部写操作单连接串行)
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("迁移失败: %w", err)
	}
	s.seed()
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }

// Exec / QueryRow 直通(单连接已串行化)
func (s *Store) Exec(query string, args ...any) (sql.Result, error) { return s.db.Exec(query, args...) }

func (s *Store) QueryRow(query string, args ...any) *sql.Row { return s.db.QueryRow(query, args...) }

func (s *Store) SetSecretKey(k []byte) { s.secretKey = k }

// ---------- 设置 ----------

func (s *Store) GetSetting(key string) (string, bool) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// ---------- SSH 凭据加解密(AES-GCM) ----------

func LoadOrCreateSecretKey(dataDir string) ([]byte, error) {
	p := filepath.Join(dataDir, "secret.key")
	if b, err := os.ReadFile(p); err == nil {
		k, err := base64.StdEncoding.DecodeString(string(b))
		if err != nil || len(k) != 32 {
			return nil, fmt.Errorf("密钥文件损坏")
		}
		return k, nil
	}
	k := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, []byte(base64.StdEncoding.EncodeToString(k)), 0o600); err != nil {
		return nil, err
	}
	return k, nil
}

func (s *Store) EncryptSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(s.secretKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

func (s *Store) DecryptSecret(enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.secretKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("密文过短")
	}
	pt, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// ---------- 重启恢复 ----------

// MarkInterruptedTasks 平台重启后,排队中/执行中任务标记「中断」(F-5.4)
func (s *Store) MarkInterruptedTasks() int {
	res, err := s.db.Exec(`UPDATE tasks SET status='interrupted', finished_at=? WHERE status IN ('queued','running')`, NowMs())
	if err != nil {
		return 0
	}
	s.db.Exec(`UPDATE test_items SET status='cancelled', error='平台重启中断', finished_at=? WHERE status IN ('pending','running')`, NowMs())
	n, _ := res.RowsAffected()
	return int(n)
}

// ---------- 备份 ----------

// BackupTo 使用 VACUUM INTO 生成一致性备份(§7.8)
func (s *Store) BackupTo(dest string) error {
	_, err := s.db.Exec(`VACUUM INTO ?`, dest)
	return err
}

// ---------- 种子数据 ----------

func (s *Store) seed() {
	// 默认全局参数
	if _, ok := s.GetSetting("parallel_limit"); !ok {
		s.SetSetting("parallel_limit", "4")
	}
	// 默认题库(常识 2 / 推理 2 / 长文摘要 1,F-3.6)
	// V2.24:出厂自带题库(29 题);按标题幂等+一次性标记,不依赖表为空(迁移 SQL 会先插文档 4 题)
	seeded := s.seedEmbeddedQuestions()
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM questions`).Scan(&n)
	if n == 0 && !seeded {
		{
			s.db.Exec(`INSERT INTO questions(qtype,title,prompt,long_text,enabled,sort) VALUES
			('common','常识:水的沸点','在标准大气压下,水的沸点是多少摄氏度?请直接回答数值并简要说明。','',1,1),
			('common','常识:中国首都','中国的首都是哪座城市?请简要介绍其地位。','',1,2),
			('reasoning','推理:年龄问题','小明今年 8 岁,他的姐姐比他大 5 岁。5 年后姐姐多少岁?请给出推理过程。','',1,3),
			('reasoning','推理:数列','观察数列 2, 6, 12, 20, 30, 下一个数是多少?请说明规律。','',1,4),
			('summary','长文摘要','请阅读以下文章并用 100 字以内概括主要内容。','PLACEHOLDER_LONG_TEXT',1,5)`)
		}
	}
	// V2.24:出厂自带公开仿真语料(corpus-public-v1,52 档);仅全新安装(表为空)时载入
	var cn int
	s.db.QueryRow(`SELECT COUNT(*) FROM corpus_versions`).Scan(&cn)
	if cn == 0 {
		s.seedEmbeddedCorpus()
	}
}

