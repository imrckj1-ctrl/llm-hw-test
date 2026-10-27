// 测试项模块注册表(V2.5《测试项扩展规范》核心)
//
// 新增测试项 = 照本文件的约定加一个模块:
//  1. 新建 item_xxx.go,实现 Run 方法(约定见规范文档第 3 节)
//  2. 在 store.ExecParams/ItemsEnabled 加参数字段
//  3. 在下面的 registry 加一行注册
//  4. 前端 TaskForm 加配置块、TaskDetail 加结果块、utils 加中文名
// 规范文档:《测试项扩展规范-V2.5.md》
package runner

import "llmhwtest/internal/store"

// TestModule 测试项模块契约
type TestModule struct {
	Name        string // 模块键:英文小写,入库 test_items.type / samples.phase 关联
	DisplayName string // 界面中文名(日志/进度/标签)
	Enabled     func(store.TaskParams) bool
	Run         func(*execContext) error
}

// registry 全部已注册测试项(顺序 = 执行顺序)
// 注意:方法值在 Runner 实例上绑定,故用包级函数持有,Runner 方法 r.modules() 包装
func (r *Runner) modules() []TestModule {
	return []TestModule{
		{"ladder", "阶梯测试", func(p store.TaskParams) bool { return p.Items.Ladder }, r.runLadder},
		{"concurrency", "并发测试", func(p store.TaskParams) bool { return p.Items.Concurrency }, r.runConcurrency},
		{"cache", "缓存测试", func(p store.TaskParams) bool { return p.Items.Cache }, r.runCache},
		{"stability", "稳定性测试", func(p store.TaskParams) bool { return p.Items.Stability }, r.runStability},
		{"quality", "能力测试", func(p store.TaskParams) bool { return p.Items.Quality }, r.runQuality},
		{"niah", "NIAH 检索测试", func(p store.TaskParams) bool { return p.Items.NIAH }, r.runNIAH},
	}
}

// moduleByName 按模块键查找(单项重跑用)
func (r *Runner) moduleByName(name string) *TestModule {
	mods := r.modules()
	for i := range mods {
		if mods[i].Name == name {
			return &mods[i]
		}
	}
	return nil
}

func (r *Runner) itemDisplayName(t string) string {
	if m := r.moduleByName(t); m != nil {
		return m.DisplayName
	}
	return t
}
