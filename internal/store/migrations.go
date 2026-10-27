package store

import "fmt"

// 内嵌 SQL migration,按版本号顺序执行(§7.1)
var migrations = []struct {
	version int
	sql     string
}{
	{1, `
CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE IF NOT EXISTS sessions(token_hash TEXT PRIMARY KEY, created_at INTEGER, expires_at INTEGER);
CREATE TABLE IF NOT EXISTS monitor_adapters(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  vendor TEXT DEFAULT '',
  command TEXT DEFAULT '',
  parse_type TEXT DEFAULT '',
  parse_rule TEXT DEFAULT '',
  builtin INTEGER DEFAULT 0,
  created_at INTEGER
);
CREATE TABLE IF NOT EXISTS devices(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  form_factor TEXT DEFAULT '',
  cpu_soc TEXT DEFAULT '',
  chip_model TEXT DEFAULT '',
  chip_count INTEGER DEFAULT 0,
  tops_int8 REAL DEFAULT 0,
  tflops_fp16 REAL DEFAULT 0,
  vram_gb REAL DEFAULT 0,
  vram_type TEXT DEFAULT '',
  vram_bw_gbps REAL DEFAULT 0,
  vram_shared INTEGER DEFAULT 0,
  host_platform TEXT DEFAULT '',
  pcie_gen TEXT DEFAULT '',
  pcie_lanes TEXT DEFAULT '',
  memory TEXT DEFAULT '',
  storage TEXT DEFAULT '',
  cooling TEXT DEFAULT '',
  tdp_w REAL DEFAULT 0,
  network TEXT DEFAULT '',
  notes TEXT DEFAULT '',
  ssh_host TEXT DEFAULT '',
  ssh_port INTEGER DEFAULT 22,
  ssh_user TEXT DEFAULT '',
  ssh_auth_type TEXT DEFAULT 'password',
  ssh_secret_enc TEXT DEFAULT '',
  monitor_adapter_id INTEGER REFERENCES monitor_adapters(id),
  manual_power_w REAL DEFAULT 0,
  archived INTEGER DEFAULT 0,
  created_at INTEGER, updated_at INTEGER
);
CREATE TABLE IF NOT EXISTS corpus_versions(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  version_no TEXT UNIQUE NOT NULL,
  uploader TEXT DEFAULT '',
  note TEXT DEFAULT '',
  active INTEGER DEFAULT 0,
  created_at INTEGER
);
CREATE TABLE IF NOT EXISTS corpus_texts(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  version_id INTEGER NOT NULL REFERENCES corpus_versions(id) ON DELETE CASCADE,
  tier TEXT NOT NULL,
  content TEXT NOT NULL,
  char_count INTEGER DEFAULT 0,
  UNIQUE(version_id, tier)
);
CREATE TABLE IF NOT EXISTS questions(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  qtype TEXT NOT NULL,
  title TEXT DEFAULT '',
  prompt TEXT NOT NULL,
  long_text TEXT DEFAULT '',
  enabled INTEGER DEFAULT 1,
  sort INTEGER DEFAULT 0
);
CREATE TABLE IF NOT EXISTS projects(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  device_id INTEGER NOT NULL REFERENCES devices(id),
  name TEXT DEFAULT '',
  tester TEXT DEFAULT '',
  note TEXT DEFAULT '',
  created_at INTEGER, started_at INTEGER, finished_at INTEGER
);
CREATE TABLE IF NOT EXISTS tasks(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  device_id INTEGER NOT NULL REFERENCES devices(id),
  seq INTEGER DEFAULT 0,
  api_base_url TEXT NOT NULL,
  model_name TEXT NOT NULL,
  model_hf_id TEXT DEFAULT '',
  model_ms_id TEXT DEFAULT '',
  quant TEXT DEFAULT '',
  context_len INTEGER DEFAULT 0,
  cache_enabled TEXT DEFAULT 'unknown',
  cache_note TEXT DEFAULT '',
  params_json TEXT NOT NULL,
  corpus_version_id INTEGER REFERENCES corpus_versions(id),
  status TEXT NOT NULL DEFAULT 'pending',
  progress INTEGER DEFAULT 0,
  current_item TEXT DEFAULT '',
  error TEXT DEFAULT '',
  run_no INTEGER DEFAULT 1,
  created_at INTEGER, started_at INTEGER, finished_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_device ON tasks(device_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE TABLE IF NOT EXISTS test_items(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  error TEXT DEFAULT '',
  started_at INTEGER, finished_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_items_task ON test_items(task_id);
CREATE TABLE IF NOT EXISTS request_samples(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  item_id INTEGER REFERENCES test_items(id) ON DELETE CASCADE,
  run_no INTEGER DEFAULT 1,
  phase TEXT NOT NULL,
  tier TEXT DEFAULT '',
  concurrency INTEGER DEFAULT 1,
  seq INTEGER DEFAULT 0,
  send_ts INTEGER, first_chunk_ts INTEGER, end_ts INTEGER,
  ttft_ms REAL DEFAULT 0,
  total_ms REAL DEFAULT 0,
  prompt_tokens INTEGER DEFAULT 0,
  completion_tokens INTEGER DEFAULT 0,
  tokens_estimated INTEGER DEFAULT 0,
  output_incomplete INTEGER DEFAULT 0,
  length_mismatch INTEGER DEFAULT 0,
  speed REAL DEFAULT 0,
  status TEXT DEFAULT 'ok',
  error TEXT DEFAULT '',
  raw_response TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_samples_task ON request_samples(task_id, run_no, phase);
CREATE INDEX IF NOT EXISTS idx_samples_item ON request_samples(item_id);
CREATE TABLE IF NOT EXISTS monitor_samples(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  run_no INTEGER DEFAULT 1,
  ts INTEGER,
  vram_used_mb REAL DEFAULT 0,
  vram_total_mb REAL DEFAULT 0,
  util_pct REAL DEFAULT 0,
  power_w REAL DEFAULT 0,
  temp_c REAL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_ms_task ON monitor_samples(task_id, run_no);
CREATE TABLE IF NOT EXISTS quality_answers(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  run_no INTEGER DEFAULT 1,
  question_id INTEGER REFERENCES questions(id),
  question_title TEXT DEFAULT '',
  answer_text TEXT DEFAULT '',
  auto_flag TEXT DEFAULT '',
  auto_reason TEXT DEFAULT '',
  manual_flag TEXT DEFAULT '',
  manual_note TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_qa_task ON quality_answers(task_id, run_no);
CREATE TABLE IF NOT EXISTS task_logs(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  ts INTEGER, level TEXT DEFAULT 'info', message TEXT
);
CREATE INDEX IF NOT EXISTS idx_logs_task ON task_logs(task_id, id);
`},
	{2, `
ALTER TABLE test_items ADD COLUMN run_no INTEGER NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS idx_items_task_run ON test_items(task_id, run_no);
`},
	{3, `
CREATE TABLE IF NOT EXISTS device_files(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  device_id INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  filename TEXT NOT NULL,
  content_type TEXT DEFAULT '',
  size INTEGER DEFAULT 0,
  data BLOB,
  created_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_device_files ON device_files(device_id);
`},
	{4, `
ALTER TABLE tasks ADD COLUMN api_key_enc TEXT DEFAULT '';
`},
	{5, `
-- V2.2:纯模型测试(项目类型 + 无设备)+ 能力题库 + 评判字段
ALTER TABLE projects RENAME TO projects_old;
CREATE TABLE projects(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL DEFAULT 'hardware',   -- hardware/model
  device_id INTEGER REFERENCES devices(id), -- 纯模型项目为 NULL
  name TEXT DEFAULT '',
  tester TEXT DEFAULT '',
  note TEXT DEFAULT '',
  created_at INTEGER, started_at INTEGER, finished_at INTEGER
);
INSERT INTO projects(id,type,device_id,name,tester,note,created_at,started_at,finished_at)
  SELECT id,'hardware',device_id,name,tester,note,created_at,started_at,finished_at FROM projects_old;
DROP TABLE projects_old;
ALTER TABLE questions ADD COLUMN category TEXT DEFAULT '';
ALTER TABLE questions ADD COLUMN expected_answer TEXT DEFAULT '';
ALTER TABLE questions ADD COLUMN judge_mode TEXT DEFAULT 'llm';
ALTER TABLE questions ADD COLUMN artifact_type TEXT DEFAULT 'none';
ALTER TABLE quality_answers ADD COLUMN judge_score TEXT DEFAULT '';
ALTER TABLE quality_answers ADD COLUMN judge_reason TEXT DEFAULT '';
ALTER TABLE quality_answers ADD COLUMN artifact_path TEXT DEFAULT '';
ALTER TABLE quality_answers ADD COLUMN artifact_ok INTEGER DEFAULT 0;
UPDATE questions SET category=qtype WHERE category='';
-- tasks.device_id 放宽为可空(纯模型任务无设备)
ALTER TABLE tasks RENAME TO tasks_old;
CREATE TABLE tasks(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  device_id INTEGER REFERENCES devices(id),
  seq INTEGER DEFAULT 0,
  api_base_url TEXT NOT NULL,
  model_name TEXT NOT NULL,
  model_hf_id TEXT DEFAULT '',
  model_ms_id TEXT DEFAULT '',
  quant TEXT DEFAULT '',
  context_len INTEGER DEFAULT 0,
  cache_enabled TEXT DEFAULT 'unknown',
  cache_note TEXT DEFAULT '',
  api_key_enc TEXT DEFAULT '',
  params_json TEXT NOT NULL,
  corpus_version_id INTEGER REFERENCES corpus_versions(id),
  status TEXT NOT NULL DEFAULT 'pending',
  progress INTEGER DEFAULT 0,
  current_item TEXT DEFAULT '',
  error TEXT DEFAULT '',
  run_no INTEGER DEFAULT 1,
  created_at INTEGER, started_at INTEGER, finished_at INTEGER
);
INSERT INTO tasks SELECT id,project_id,device_id,seq,api_base_url,model_name,model_hf_id,model_ms_id,
  quant,context_len,cache_enabled,cache_note,api_key_enc,params_json,corpus_version_id,status,progress,
  current_item,error,run_no,created_at,started_at,finished_at FROM tasks_old;
DROP TABLE tasks_old;
CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_device ON tasks(device_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
`},
	{6, `
-- V2.2 简化:硬件信息随项目快照 + 规格书挂项目
ALTER TABLE projects ADD COLUMN hw_json TEXT DEFAULT '';
CREATE TABLE IF NOT EXISTS project_files(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  filename TEXT NOT NULL,
  content_type TEXT DEFAULT '',
  size INTEGER DEFAULT 0,
  data BLOB,
  created_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_project_files ON project_files(project_id);
`},
	{7, `
-- 修复 v5 重建遗留:子表外键被改写到 tasks_old,需重建为指向 tasks
-- (migrate() 已在迁移期间关闭外键,此迁移安全执行)
ALTER TABLE test_items RENAME TO test_items_bak;
CREATE TABLE test_items(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  run_no INTEGER NOT NULL DEFAULT 1,
  type TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  error TEXT DEFAULT '',
  started_at INTEGER, finished_at INTEGER
);
INSERT INTO test_items SELECT id,task_id,run_no,type,status,error,started_at,finished_at FROM test_items_bak;
DROP TABLE test_items_bak;
CREATE INDEX IF NOT EXISTS idx_items_task ON test_items(task_id);
CREATE INDEX IF NOT EXISTS idx_items_task_run ON test_items(task_id, run_no);

ALTER TABLE request_samples RENAME TO request_samples_bak;
CREATE TABLE request_samples(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  item_id INTEGER REFERENCES test_items(id) ON DELETE CASCADE,
  run_no INTEGER DEFAULT 1,
  phase TEXT NOT NULL,
  tier TEXT DEFAULT '',
  concurrency INTEGER DEFAULT 1,
  seq INTEGER DEFAULT 0,
  send_ts INTEGER, first_chunk_ts INTEGER, end_ts INTEGER,
  ttft_ms REAL DEFAULT 0,
  total_ms REAL DEFAULT 0,
  prompt_tokens INTEGER DEFAULT 0,
  completion_tokens INTEGER DEFAULT 0,
  tokens_estimated INTEGER DEFAULT 0,
  output_incomplete INTEGER DEFAULT 0,
  length_mismatch INTEGER DEFAULT 0,
  speed REAL DEFAULT 0,
  status TEXT DEFAULT 'ok',
  error TEXT DEFAULT '',
  raw_response TEXT DEFAULT ''
);
INSERT INTO request_samples SELECT * FROM request_samples_bak;
DROP TABLE request_samples_bak;
CREATE INDEX IF NOT EXISTS idx_samples_task ON request_samples(task_id, run_no, phase);
CREATE INDEX IF NOT EXISTS idx_samples_item ON request_samples(item_id);

ALTER TABLE monitor_samples RENAME TO monitor_samples_bak;
CREATE TABLE monitor_samples(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  run_no INTEGER DEFAULT 1,
  ts INTEGER,
  vram_used_mb REAL DEFAULT 0,
  vram_total_mb REAL DEFAULT 0,
  util_pct REAL DEFAULT 0,
  power_w REAL DEFAULT 0,
  temp_c REAL DEFAULT 0
);
INSERT INTO monitor_samples SELECT * FROM monitor_samples_bak;
DROP TABLE monitor_samples_bak;
CREATE INDEX IF NOT EXISTS idx_ms_task ON monitor_samples(task_id, run_no);

ALTER TABLE quality_answers RENAME TO quality_answers_bak;
CREATE TABLE quality_answers(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  run_no INTEGER DEFAULT 1,
  question_id INTEGER REFERENCES questions(id),
  question_title TEXT DEFAULT '',
  answer_text TEXT DEFAULT '',
  auto_flag TEXT DEFAULT '',
  auto_reason TEXT DEFAULT '',
  manual_flag TEXT DEFAULT '',
  manual_note TEXT DEFAULT '',
  judge_score TEXT DEFAULT '',
  judge_reason TEXT DEFAULT '',
  artifact_path TEXT DEFAULT '',
  artifact_ok INTEGER DEFAULT 0
);
INSERT INTO quality_answers SELECT * FROM quality_answers_bak;
DROP TABLE quality_answers_bak;
CREATE INDEX IF NOT EXISTS idx_qa_task ON quality_answers(task_id, run_no);

ALTER TABLE task_logs RENAME TO task_logs_bak;
CREATE TABLE task_logs(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  ts INTEGER, level TEXT DEFAULT 'info', message TEXT
);
INSERT INTO task_logs SELECT * FROM task_logs_bak;
DROP TABLE task_logs_bak;
CREATE INDEX IF NOT EXISTS idx_logs_task ON task_logs(task_id, id);
`},
	{8, `
-- 思维链计时与 usage 护栏标记(修改清单 V2.2 补充)
ALTER TABLE request_samples ADD COLUMN usage_suspect INTEGER DEFAULT 0;
ALTER TABLE request_samples ADD COLUMN reasoning_only INTEGER DEFAULT 0;
`},
	{9, `
-- V2.3:请求体去重存储(全量日志)、缓存命中、并行默认 5
CREATE TABLE IF NOT EXISTS request_bodies(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  hash TEXT UNIQUE,
  body TEXT
);
ALTER TABLE request_samples ADD COLUMN request_id INTEGER DEFAULT 0;
ALTER TABLE request_samples ADD COLUMN cached_tokens INTEGER DEFAULT 0;
UPDATE settings SET value='5' WHERE key='parallel_limit' AND value='4';
`},
	{10, `
CREATE TABLE IF NOT EXISTS question_groups(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  note TEXT DEFAULT '',
  created_at INTEGER
);
CREATE TABLE IF NOT EXISTS question_group_items(
  group_id INTEGER NOT NULL REFERENCES question_groups(id) ON DELETE CASCADE,
  question_id INTEGER NOT NULL,
  UNIQUE(group_id, question_id)
);
CREATE INDEX IF NOT EXISTS idx_qgi_group ON question_group_items(group_id);
`},
	{11, `
ALTER TABLE request_samples ADD COLUMN meta TEXT DEFAULT '';
INSERT INTO questions(qtype,title,prompt,long_text,enabled,sort,category,expected_answer,judge_mode,artifact_type)
SELECT 'doc','文档:季度销售 Excel','生成一份 2026 年第一季度销售数据表:包含 产品名称、1月、2月、3月、季度合计 五列,至少 6 行产品数据,数值合理且合计正确。','',1,20,'doc','含表头与至少 6 行数据,合计列正确','llm','xlsx'
WHERE NOT EXISTS(SELECT 1 FROM questions WHERE title='文档:季度销售 Excel');
INSERT INTO questions(qtype,title,prompt,long_text,enabled,sort,category,expected_answer,judge_mode,artifact_type)
SELECT 'doc','文档:产品介绍 PPT','生成一份 3 页的产品介绍 PPT,主题为「灵思微致算力卡」:第 1 页封面(标题+副标题),第 2 页核心参数(至少 4 条要点),第 3 页总结与联系方式。','',1,21,'doc','3 页结构完整,要点清晰','llm','pptx'
WHERE NOT EXISTS(SELECT 1 FROM questions WHERE title='文档:产品介绍 PPT');
INSERT INTO questions(qtype,title,prompt,long_text,enabled,sort,category,expected_answer,judge_mode,artifact_type)
SELECT 'doc','文档:项目周报 Word','生成一份项目周报 Word 文档,包含 标题、本周进展(至少 3 条)、风险与问题、下周计划 四个部分,内容具体不空洞。','',1,22,'doc','四部分齐全,内容具体','llm','docx'
WHERE NOT EXISTS(SELECT 1 FROM questions WHERE title='文档:项目周报 Word');
INSERT INTO questions(qtype,title,prompt,long_text,enabled,sort,category,expected_answer,judge_mode,artifact_type)
SELECT 'doc','文档:测试报告 PDF','生成一份模型测试报告 PDF,包含 标题、测试概述、关键指标(至少 3 条)、结论 四个部分,排版清晰。','',1,23,'doc','四部分齐全,排版清晰','llm','pdf'
WHERE NOT EXISTS(SELECT 1 FROM questions WHERE title='文档:测试报告 PDF');
`},
	{13, `
ALTER TABLE questions ADD COLUMN use_goal INTEGER DEFAULT 0;
UPDATE questions SET use_goal=1 WHERE artifact_type IN ('html','code','xlsx','pptx','docx','pdf');
`},
	{14, `
ALTER TABLE quality_answers ADD COLUMN artifact_type TEXT DEFAULT '';
`},
	{15, `
-- V2.12:任务级语料/题目快照(任务终态时冻结,数据包从此读,不再读题库/语料库现值)
CREATE TABLE IF NOT EXISTS task_corpus_snapshot(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL,
  run_no INTEGER NOT NULL,
  version_no TEXT DEFAULT '',
  tier TEXT NOT NULL,
  content TEXT NOT NULL,
  char_count INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_tcs_task ON task_corpus_snapshot(task_id, run_no);
CREATE TABLE IF NOT EXISTS task_question_snapshot(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL,
  run_no INTEGER NOT NULL,
  question_id INTEGER DEFAULT 0,
  title TEXT DEFAULT '',
  prompt TEXT DEFAULT '',
  long_text TEXT DEFAULT '',
  qtype TEXT DEFAULT '',
  category TEXT DEFAULT '',
  expected_answer TEXT DEFAULT '',
  judge_mode TEXT DEFAULT '',
  artifact_type TEXT DEFAULT '',
  use_goal INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_tqs_task ON task_question_snapshot(task_id, run_no);
`},
	{16, `
-- V3.0.1:项目隐藏(仅隐藏不删除,列表/对比分析/首页不可见,数据保留可恢复)
ALTER TABLE projects ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0;
`},
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at INTEGER)`); err != nil {
		return err
	}
	// 重建表类迁移必须关闭外键约束,否则 RENAME 会改写子表引用导致断链;
	// PRAGMA 不能在事务内切换,故在循环外设置
	s.db.Exec(`PRAGMA foreign_keys=OFF`)
	defer s.db.Exec(`PRAGMA foreign_keys=ON`)
	var cur int
	s.db.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&cur)
	for _, m := range migrations {
		if m.version <= cur {
			continue
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration v%d: %w", m.version, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES(?,?)`, m.version, NowMs()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
