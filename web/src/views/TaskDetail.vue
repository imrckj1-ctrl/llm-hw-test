<template>
  <div v-loading="!task">
    <template v-if="task">
      <BackBar :title="`任务 ${task.id} · ${task.model_name}`">
        <el-tag size="small" effect="plain">{{ task.quant || '未填精度' }}</el-tag>
        <div>
          <el-button @click="openPackage"><el-icon><FolderOpened /></el-icon>数据包</el-button>
          <el-button v-if="['interrupted','cancelled','failed','paused'].includes(task.status)" type="primary" @click="resume">继续跑</el-button>
          <template v-if="task.status === 'running'">
            <el-button type="warning" plain @click="pauseTask">暂停任务</el-button>
            <el-button type="warning" plain @click="skipItem">跳过当前项</el-button>
          </template>
          <el-button v-if="['queued','running'].includes(task.status)" type="warning" @click="cancel">取消任务</el-button>
          <el-button v-else-if="task.status !== 'done'" type="success" @click="restart">重跑任务</el-button>
          <el-button v-if="['pending','failed','cancelled','interrupted','paused'].includes(task.status)" @click="openEdit"><el-icon><Edit /></el-icon>编辑任务</el-button>
          <el-button v-if="task.status === 'done'" plain @click="copyTask"><el-icon><CopyDocument /></el-icon>复制为新任务</el-button>
          <el-button @click="downloadLog"><el-icon><Download /></el-icon>日志</el-button>
        </div>
      </BackBar>

      <el-card v-if="project" shadow="never" style="margin-bottom: 14px">
        <el-descriptions :column="4" size="small" border>
          <el-descriptions-item label="项目">{{ project.name }}</el-descriptions-item>
          <el-descriptions-item label="类型">
            <el-tag size="small" effect="plain">{{ project.type === 'model' ? '纯模型测试' : '本地硬件' }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="测试人员">{{ project.tester || '-' }}</el-descriptions-item>
          <el-descriptions-item label="创建时间">{{ fmtTime(project.created_at) }}</el-descriptions-item>
          <el-descriptions-item label="备注" :span="4">{{ project.note || '-' }}</el-descriptions-item>
        </el-descriptions>
        <template v-if="project.type === 'hardware' && project.hw_json">
          <el-divider content-position="left">硬件信息</el-divider>
          <el-descriptions :column="4" size="small" border>
            <el-descriptions-item label="形态">{{ hwInfo.form_factor || '-' }}</el-descriptions-item>
            <el-descriptions-item label="主控">{{ hwInfo.cpu_soc || '-' }}</el-descriptions-item>
            <el-descriptions-item label="AI 芯片">{{ hwInfo.chip_model || '-' }}{{ hwInfo.chip_count > 1 ? ' ×' + hwInfo.chip_count : '' }}</el-descriptions-item>
            <el-descriptions-item label="算力">{{ hwInfo.tops_int8 || 0 }} TOPS / {{ hwInfo.tflops_fp16 || 0 }} TFLOPS</el-descriptions-item>
            <el-descriptions-item label="显存">{{ hwInfo.vram_gb || 0 }}GB {{ hwInfo.vram_type }}</el-descriptions-item>
            <el-descriptions-item label="固件版本">{{ hwInfo.firmware || '-' }}</el-descriptions-item>
            <el-descriptions-item label="散热/TDP">{{ hwInfo.cooling || '-' }} / {{ hwInfo.tdp_w || 0 }}W</el-descriptions-item>
            <el-descriptions-item label="硬件备注">{{ hwInfo.notes || '-' }}</el-descriptions-item>
          </el-descriptions>
          <template v-if="hwInfo.machine_form || hwInfo.machine_model || hwInfo.machine_cpu || hwInfo.memory || hwInfo.storage || hwInfo.os_version">
            <el-divider content-position="left">测试整机信息</el-divider>
            <el-descriptions :column="4" size="small" border>
              <el-descriptions-item label="整机形态">{{ hwInfo.machine_form || '-' }}</el-descriptions-item>
              <el-descriptions-item label="整机型号">{{ hwInfo.machine_model || '-' }}</el-descriptions-item>
              <el-descriptions-item label="整机 CPU">{{ hwInfo.machine_cpu || '-' }}</el-descriptions-item>
              <el-descriptions-item label="操作系统">{{ hwInfo.os_version || '-' }}</el-descriptions-item>
              <el-descriptions-item label="内存/硬盘" :span="2">{{ hwInfo.memory || '-' }} / {{ hwInfo.storage || '-' }}</el-descriptions-item>
            </el-descriptions>
          </template>
          <template v-if="hwInfo.framework || hwInfo.driver_ver || hwInfo.sdk_ver || hwInfo.startup_cmd || hwInfo.env_note">
            <el-divider content-position="left">推理软件环境</el-divider>
            <el-descriptions :column="4" size="small" border>
              <el-descriptions-item label="推理框架">{{ hwInfo.framework || '-' }}</el-descriptions-item>
              <el-descriptions-item label="驱动版本">{{ hwInfo.driver_ver || '-' }}</el-descriptions-item>
              <el-descriptions-item label="SDK / 算子库" :span="2">{{ hwInfo.sdk_ver || '-' }}</el-descriptions-item>
              <el-descriptions-item label="启动命令" :span="4"><code style="font-size: 12px; word-break: break-all">{{ hwInfo.startup_cmd || '-' }}</code></el-descriptions-item>
              <el-descriptions-item v-if="hwInfo.env_note" label="备注说明" :span="4">{{ hwInfo.env_note }}</el-descriptions-item>
            </el-descriptions>
          </template>
          <div style="margin-top: 10px">
            <el-upload :auto-upload="false" :show-file-list="false" :on-change="onPickSpec" style="display: inline-block">
              <el-button size="small" plain><el-icon><Upload /></el-icon>上传规格书</el-button>
            </el-upload>
            <div v-for="f in specFiles" :key="f.id" class="spec-row">
              <el-icon><Document /></el-icon>
              <span class="spec-name">{{ f.filename }}</span>
              <span class="spec-meta">{{ fmtSize(f.size) }} · {{ fmtTime(f.created_at) }}</span>
              <el-button size="small" text type="primary" @click="downloadSpec(f)">下载</el-button>
              <el-popconfirm title="删除该附件?" @confirm="removeSpec(f)">
                <template #reference><el-button size="small" text type="danger">删除</el-button></template>
              </el-popconfirm>
            </div>
          </div>
        </template>
      </el-card>

      <el-card shadow="never" style="margin-bottom: 14px">
        <div style="display: flex; gap: 24px; align-items: center; flex-wrap: wrap">
          <el-tag :type="taskStatus[task.status]?.type" size="large">{{ taskStatus[task.status]?.text }}</el-tag>
          <el-progress :percentage="task.progress" :stroke-width="14" style="flex: 1; min-width: 220px" />
          <span v-if="task.current_item" style="color: #606266">当前阶段:<b>{{ task.current_item }}</b></span>
          <span style="color: #909399; display: inline-flex; align-items: center; gap: 6px">
            run
            <el-select v-model="selectedRun" size="small" style="width: 100px" @change="onRunChange">
              <el-option v-for="rn in runs" :key="rn" :label="String(rn)" :value="rn" />
            </el-select>
            <el-tag v-if="selectedRun !== task.run_no" size="small" type="warning" effect="plain">历史数据</el-tag>
            · 语料 {{ task.corpus_version_no || '-' }}
          </span>
        </div>
        <el-alert v-if="task.error" type="error" :title="task.error" :closable="false" style="margin-top: 10px" />
        <div style="margin-top: 10px; color: #909399; font-size: 13px">
          {{ task.api_base_url }} · 上下文 {{ task.context_len ? formatContextLen(task.context_len) : '未填' }} · 缓存开关 {{ cacheCN[task.cache_enabled] }}
          <span v-if="task.cache_note"> · {{ task.cache_note }}</span>
        </div>
      </el-card>

      <el-row :gutter="10" style="margin-bottom: 14px">
        <el-col :span="4" v-if="hasLadder"><el-card shadow="never" class="metric-card">
          <div class="metric-value">{{ fmtNum(summary.speed_1k) }}</div>
          <div class="metric-label">1K 输出速度 (tokens/s)</div>
        </el-card></el-col>
        <el-col :span="4" v-if="hasLadder"><el-card shadow="never" class="metric-card">
          <div class="metric-value">{{ fmtNum(summary.top_tier_ttft, 0) }}</div>
          <div class="metric-label">{{ summary.top_tier || '-' }} TTFT (ms)</div>
        </el-card></el-col>
        <el-col :span="4" v-if="hasConcurrency"><el-card shadow="never" class="metric-card">
          <div class="metric-value">{{ fmtNum(summary.peak_throughput) }}</div>
          <div class="metric-label">峰值总吞吐(并发 {{ summary.peak_concurrency || '-' }})</div>
        </el-card></el-col>
        <el-col :span="4" v-if="hasStability"><el-card shadow="never" class="metric-card">
          <div class="metric-value">{{ summary.decay_tested ? summary.degradation.toFixed(1) + '%' : '-' }}</div>
          <div class="metric-label">性能衰减</div>
        </el-card></el-col>
        <el-col :span="4" v-if="hasQuality"><el-card shadow="never" class="metric-card">
          <div class="metric-value">{{ capOverall !== null ? capOverall.toFixed(2) : '-' }}</div>
          <div class="metric-label">能力总分(0~1)</div>
        </el-card></el-col>
        <el-col :span="4" v-if="hasQuality"><el-card shadow="never" class="metric-card">
          <div class="metric-value">
            <el-tag :type="qualityType">{{ summary.quality || '未测试' }}</el-tag>
          </div>
          <div class="metric-label">质量结论</div>
        </el-card></el-col>
        <el-col :span="4"><el-card shadow="never" class="metric-card">
          <div class="metric-value">{{ fmtInt(summary.total_prompt) }}</div>
          <div class="metric-label">总输入 tokens</div>
        </el-card></el-col>
        <el-col :span="4"><el-card shadow="never" class="metric-card">
          <div class="metric-value">{{ fmtInt(summary.total_completion) }}</div>
          <div class="metric-label">总输出 tokens</div>
        </el-card></el-col>
        <el-col :span="4"><el-card shadow="never" class="metric-card">
          <div class="metric-value">{{ fmtInt(summary.total_calls) }}</div>
          <div class="metric-label">总调用次数(成功 {{ fmtInt(summary.ok_calls) }} / 失败 {{ fmtInt(summary.fail_calls) }})</div>
        </el-card></el-col>
      </el-row>

      <el-tabs v-model="activeTab">
        <el-tab-pane label="结果" name="results">
          <template v-for="item in items" :key="item.id">
            <el-card shadow="never" style="margin-bottom: 14px">
              <template #header>
                <div style="display: flex; justify-content: space-between; align-items: center">
                  <span>
                    <b>{{ itemTypeName[item.type] }}</b>
                    <el-tag size="small" :type="itemStatus[item.status]?.type" style="margin-left: 8px">
                      {{ itemStatus[item.status]?.text }}
                    </el-tag>
                    <span v-if="item.error" style="color: #f56c6c; font-size: 12px; margin-left: 8px">{{ item.error }}</span>
                  </span>
                  <span style="white-space: nowrap">
                    <el-tooltip content="暂停任务(中止当前请求,可断点续跑)" placement="top">
                      <el-button size="small" text type="warning" :disabled="item.status !== 'running'"
                        @click="pauseTask">暂停</el-button>
                    </el-tooltip>
                    <el-tooltip content="继续(中断的项断点续跑;跳过的项重新执行)" placement="top">
                      <el-button size="small" text type="success" :disabled="!canContinueItem(item)"
                        @click="continueItem(item)">继续</el-button>
                    </el-tooltip>
                    <el-tooltip content="跳过该测试项(之后可点「继续」重跑)" placement="top">
                      <el-button size="small" text :disabled="!canSkipItem(item)"
                        @click="skipItemById(item)">跳过</el-button>
                    </el-tooltip>
                    <el-tooltip content="单项重跑(清空该项数据,从头完整重测)" placement="top">
                      <el-button size="small" text type="primary" :disabled="['running','queued'].includes(task.status)"
                        @click="rerunItem(item)">单项重跑</el-button>
                    </el-tooltip>
                  </span>
                </div>
              </template>

              <template v-if="item.type === 'ladder'">
                <el-table :data="results.ladder" border size="small" style="margin-bottom: 10px">
                  <el-table-column prop="tier" label="档位" width="70" />
                  <el-table-column prop="prompt_tokens" label="实际 tokens" width="95" />
                  <el-table-column label="TTFT(ms)" align="center">
                    <el-table-column v-for="s in statCols" :key="'lt' + s.k" :label="s.label" :width="s.w">
                      <template #default="{ row }">{{ row.skipped ? '-' : fmtNum(row.ttft?.[s.k], 1) }}</template>
                    </el-table-column>
                  </el-table-column>
                  <el-table-column label="速度(tokens/s)" align="center">
                    <el-table-column v-for="s in statCols" :key="'ls' + s.k" :label="s.label" :width="s.w">
                      <template #default="{ row }">{{ row.skipped ? '-' : fmtNum(row.speed?.[s.k], 2) }}</template>
                    </el-table-column>
                  </el-table-column>
                  <el-table-column label="ITL(ms)" width="85" align="center">
                    <template #default="{ row }">{{ row.skipped ? '-' : fmtITL(row.speed?.avg) }}</template>
                  </el-table-column>
                  <el-table-column prop="samples" label="样本数" min-width="90" />
                  <el-table-column label="进度" width="80" align="center">
                    <template #default="{ row }">
                      <span v-if="row.skipped">-</span>
                      <b v-else :style="row.samples >= row.planned ? 'color:#67c23a' : 'color:#e6a23c'">{{ row.samples }}/{{ row.planned }}</b>
                    </template>
                  </el-table-column>
                  <el-table-column label="成功" width="80" align="center">
                    <template #default="{ row }">
                      <span v-if="row.skipped">-</span>
                      <span v-else>{{ row.ok_count }}/{{ row.samples }}</span>
                    </template>
                  </el-table-column>
                </el-table>
                <el-row :gutter="10">
                  <el-col :span="12"><Chart :option="ladderSpeedChart" :height="260" /></el-col>
                  <el-col :span="12"><Chart :option="ladderTtftChart" :height="260" /></el-col>
                </el-row>
              </template>

              <template v-else-if="item.type === 'concurrency'">
                <el-table :data="results.concurrency" border size="small" style="margin-bottom: 10px" :span-method="concSpan">
                  <el-table-column label="输入档" width="80" align="center">
                    <template #default="{ row }"><b>{{ row.tier || '1K' }}</b></template>
                  </el-table-column>
                  <el-table-column label="并发档" width="70" align="center">
                    <template #default="{ row }"><b>{{ row.level }}</b></template>
                  </el-table-column>
                  <el-table-column label="进度(轮)" width="85" align="center">
                    <template #default="{ row }">
                      <span v-if="row.circuit_trip">熔断跳过</span>
                      <b v-else :style="row.rounds >= row.planned_rounds ? 'color:#67c23a' : 'color:#e6a23c'">{{ row.rounds }}/{{ row.planned_rounds }}</b>
                    </template>
                  </el-table-column>
                  <el-table-column label="成功/失败" width="90" align="center">
                    <template #default="{ row }">
                      <span v-if="row.circuit_trip">-</span>
                      <span v-else>{{ row.ok_count }}/{{ row.fail_count }}</span>
                    </template>
                  </el-table-column>
                  <el-table-column label="失败率" width="80">
                    <template #default="{ row }">{{ row.circuit_trip ? '-' : (row.fail_rate * 100).toFixed(1) + '%' }}</template>
                  </el-table-column>
                  <el-table-column label="TTFT(ms)" align="center">
                    <el-table-column v-for="s in statCols" :key="'ct' + s.k" :label="s.label" :width="s.w">
                      <template #default="{ row }">{{ row.circuit_trip ? '-' : fmtNum(row.ttft?.[s.k], 1) }}</template>
                    </el-table-column>
                  </el-table-column>
                  <el-table-column label="单请求速度(tokens/s)" align="center">
                    <el-table-column v-for="s in statCols" :key="'cs' + s.k" :label="s.label" :width="s.w">
                      <template #default="{ row }">{{ row.circuit_trip ? '-' : fmtNum(row.speed?.[s.k], 2) }}</template>
                    </el-table-column>
                  </el-table-column>
                  <el-table-column label="ITL(ms)" width="85" align="center">
                    <template #default="{ row }">{{ row.circuit_trip ? '-' : fmtITL(row.speed?.avg) }}</template>
                  </el-table-column>
                </el-table>
                <el-row :gutter="10">
                  <el-col :span="12"><Chart :option="concurrencyChartData" :height="260" /></el-col>
                  <el-col :span="12"><Chart :option="concurrencyTtftChart" :height="260" /></el-col>
                </el-row>
              </template>

              <template v-else-if="item.type === 'cache'">
                <el-table :data="results.cache" border size="small" style="margin-bottom: 10px">
                  <el-table-column prop="tier" label="档位" width="80" />
                  <el-table-column label="TTFT(冷,ms)" align="center">
                    <el-table-column v-for="s in statCols" :key="'cat' + s.k" :label="s.label" :width="s.w">
                      <template #default="{ row }">{{ fmtNum(row.ttft?.[s.k], 1) }}</template>
                    </el-table-column>
                  </el-table-column>
                  <el-table-column label="速度(tokens/s)" align="center">
                    <el-table-column v-for="s in statCols" :key="'cas' + s.k" :label="s.label" :width="s.w">
                      <template #default="{ row }">{{ fmtNum(row.speed?.[s.k], 2) }}</template>
                    </el-table-column>
                  </el-table-column>
                  <el-table-column label="ITL(ms)" width="85" align="center">
                    <template #default="{ row }">{{ fmtITL(row.speed?.avg) }}</template>
                  </el-table-column>
                  <el-table-column label="命中率" width="130" align="center">
                    <template #default="{ row }">
                      <template v-if="row.hit_rate > 0">
                        <b style="color: #67c23a">{{ (row.hit_rate * 100).toFixed(1) }}%</b>
                        <div v-if="row.improvement" style="font-size: 11px; color: #909399">热 TTFT 提升 {{ row.improvement.toFixed(1) }}x</div>
                      </template>
                      <span v-else>-</span>
                    </template>
                  </el-table-column>
                </el-table>
                <Chart :option="cacheChart" :height="260" />
              </template>

              <template v-else-if="item.type === 'stability'">
                <div v-if="stabParamsText" style="margin-bottom: 8px; font-size: 12px; color: #5a6472; background: #f4f6f9; border-radius: 6px; padding: 6px 10px; display: inline-block">
                  {{ stabParamsText }}
                </div>
                <el-table :data="results.stability?.windows || []" border size="small" style="margin-bottom: 10px">
                  <el-table-column label="时间段" width="190">
                    <template #default="{ row }">{{ fmtTimeShort(row.start_ts) }} – {{ fmtTimeShort(row.end_ts) }}</template>
                  </el-table-column>
                  <el-table-column label="成功 / 失败" width="110" align="center">
                    <template #default="{ row }">
                      <b style="color: #67c23a">{{ row.ok_count }}</b> / <b :style="{ color: row.fail_count ? '#f56c6c' : '#909399' }">{{ row.fail_count }}</b>
                    </template>
                  </el-table-column>
                  <el-table-column label="TTFT(ms)" align="center">
                    <el-table-column v-for="s in statCols" :key="'st' + s.k" :label="s.label" :width="s.w">
                      <template #default="{ row }">{{ fmtNum(row.ttft?.[s.k], 1) }}</template>
                    </el-table-column>
                  </el-table-column>
                  <el-table-column label="速度(tokens/s)" align="center">
                    <el-table-column v-for="s in statCols" :key="'ss' + s.k" :label="s.label" :width="s.w">
                      <template #default="{ row }">{{ fmtNum(row.speed?.[s.k], 2) }}</template>
                    </el-table-column>
                  </el-table-column>
                  <el-table-column label="ITL(ms)" width="85" align="center">
                    <template #default="{ row }">{{ fmtITL(row.speed?.avg) }}</template>
                  </el-table-column>
                  <el-table-column label="趋势" width="70" align="center">
                    <template #default="{ row }">
                      <el-tag v-if="row.trend" size="small" effect="plain"
                        :type="row.trend === '升' ? 'success' : row.trend === '降' ? 'danger' : 'info'">{{ row.trend }}</el-tag>
                    </template>
                  </el-table-column>
                </el-table>
                <el-alert v-if="results.stability?.note" type="warning" :title="results.stability.note + ';衰减 ' + results.stability.decay.toFixed(1) + '%'" :closable="false" style="margin-bottom: 10px" />
                <el-row :gutter="10">
                  <el-col :span="12"><Chart :option="stabSpeedScatter" :height="260" /></el-col>
                  <el-col :span="12"><Chart :option="stabTtftScatter" :height="260" /></el-col>
                </el-row>
              </template>

              <template v-else-if="item.type === 'quality'">
                <el-table :data="quality.list || []" border size="small">
                  <el-table-column label="分类" width="100">
                    <template #default="{ row }">
                      <el-tag size="small" effect="plain">{{ catName(row.category || row.qtype) }}</el-tag>
                    </template>
                  </el-table-column>
                  <el-table-column label="题目" min-width="140">
                    <template #default="{ row }">{{ row.question_title }}</template>
                  </el-table-column>
                  <el-table-column label="回答" min-width="300">
                    <template #default="{ row }">
                      <el-popover width="500" trigger="click">
                        <template #reference>
                          <el-link type="primary">{{ (row.answer_text || '(空)').slice(0, 60) }}…</el-link>
                        </template>
                        <pre style="white-space: pre-wrap; font-size: 13px; max-height: 400px; overflow-y: auto">{{ row.answer_text }}</pre>
                      </el-popover>
                    </template>
                  </el-table-column>
                  <el-table-column label="自动初筛" width="110">
                    <template #default="{ row }">
                      <el-tag size="small" :type="row.auto_flag === 'normal' ? 'success' : 'warning'">
                        {{ row.auto_flag === 'normal' ? '正常' : '疑似异常' }}
                      </el-tag>
                      <div v-if="row.auto_reason" style="font-size: 11px; color: #909399">{{ row.auto_reason }}</div>
                    </template>
                  </el-table-column>
                  <el-table-column label="人工标记(最终结论)" width="260">
                    <template #default="{ row }">
                      <el-radio-group size="small" v-model="row._flag" @change="row._dirty = true">
                        <el-radio-button value="normal">正常</el-radio-button>
                        <el-radio-button value="abnormal">异常</el-radio-button>
                      </el-radio-group>
                      <el-tag v-if="!row.manual_flag && !row._dirty" size="small" type="info" effect="plain" style="margin-left: 6px">按初筛</el-tag>
                      <el-input v-if="row._flag === 'abnormal'" size="small" style="margin-top: 6px"
                        v-model="row._note" placeholder="异常描述" @input="row._dirty = true" />
                      <div style="margin-top: 6px; display: flex; gap: 6px; align-items: center">
                        <el-button size="small" type="primary" :disabled="!row._dirty" @click="saveQuality(row)">
                          确认标注
                        </el-button>
                        <el-button v-if="row.manual_flag" size="small" text type="info" @click="clearQuality(row)">清除标注</el-button>
                        <el-tag v-if="row._dirty" size="small" type="warning" effect="plain">未保存</el-tag>
                      </div>
                    </template>
                  </el-table-column>
                  <el-table-column label="评判" min-width="180">
                    <template #default="{ row }">
                      <el-tag v-if="row.judge_score" size="small"
                        :type="{ pass: 'success', partial: 'warning', fail: 'danger' }[row.judge_score]">
                        {{ { pass: '对', partial: '部分对', fail: '错' }[row.judge_score] }}
                      </el-tag>
                      <div v-if="row.judge_reason" style="font-size: 11px; color: #909399; margin-top: 2px">{{ row.judge_reason }}</div>
                      <div v-if="row.artifact_path" style="margin-top: 4px">
                        <el-button v-if="['html', 'code', 'none', ''].includes(row.artifact_type)" size="small" plain type="primary" @click="openArtifact(row)">
                          <el-icon><View /></el-icon>查看产物
                        </el-button>
                        <el-button v-else size="small" plain type="success" @click="downloadArtifact(row)">
                          <el-icon><Download /></el-icon>下载产物({{ row.artifact_type }})
                        </el-button>
                      </div>
                    </template>
                  </el-table-column>
                </el-table>
              </template>

              <template v-else-if="item.type === 'niah'">
                <el-alert v-if="results.niah_effective" type="success" :closable="false" style="margin-bottom: 10px">
                  <template v-if="results.niah_effective.at_ceiling && niahMode === 'verify'">
                    与宣称一致:全部长度点位 × 埋针深度均命中(已达宣称上下文 {{ fmtTokens(results.niah_effective.tokens) }})
                  </template>
                  <template v-else-if="results.niah_effective.at_ceiling">
                    实测有效上下文 ≥ {{ fmtTokens(results.niah_effective.tokens) }}(已达探测上限,各点位均命中)
                  </template>
                  <template v-else>
                    实测有效上下文约 <b>{{ fmtTokens(results.niah_effective.tokens) }}</b>
                    <template v-if="results.niah_effective.pct">(任务上下文长度的 {{ results.niah_effective.pct }}%)</template>
                  </template>
                </el-alert>
                <!-- V2.8 扫描模式:横轴=输入长度,一眼看到「捞得到→捞不到」的边界 -->
                <template v-if="niahIsScan">
                  <el-table :data="niahScanRows" border size="small" style="margin-bottom: 10px">
                    <el-table-column label="输入长度" width="120" align="center">
                      <template #default="{ row }"><b>{{ row.tier }}</b></template>
                    </el-table-column>
                    <el-table-column :label="niahMode === 'discover' ? '点位' : '占上下文'" width="90" align="center">
                      <template #default="{ row }">{{ niahMode === 'discover' ? row.pct + 'K' : row.pct + '%' }}</template>
                    </el-table-column>
                    <el-table-column label="埋针深度" width="90" align="center">
                      <template #default="{ row }">{{ row.depth_pct }}%</template>
                    </el-table-column>
                    <el-table-column label="命中率" min-width="130" align="center">
                      <template #default="{ row }">
                        <el-tag size="small" :type="hitType(row.hit_rate)" effect="plain">
                          {{ (row.hit_rate * 100).toFixed(0) }}%({{ row.hits }}/{{ row.probes }} 次)
                        </el-tag>
                        <el-tag v-if="row.errors > 0" size="small" type="info" effect="plain" style="margin-left: 4px">错误 {{ row.errors }}</el-tag>
                      </template>
                    </el-table-column>
                    <el-table-column label="TTFT 中位" width="110" align="center">
                      <template #default="{ row }">{{ row.ttft_median > 0 ? row.ttft_median.toFixed(0) + ' ms' : '-' }}</template>
                    </el-table-column>
                    <el-table-column label="结论" width="110" align="center">
                      <template #default="{ row }">
                        <el-tag size="small" :type="row.hit_rate >= 0.5 ? 'success' : 'danger'">
                          {{ row.hit_rate >= 0.5 ? '能找回' : '找不回' }}
                        </el-tag>
                      </template>
                    </el-table-column>
                  </el-table>
                  <Chart :option="niahScanChart" :height="280" />
                </template>
                <Chart v-if="!niahIsScan && niahMatrix.length" :option="niahHeatmap" :height="niahHeatHeight" />
                <el-table v-if="!niahIsScan" :data="niahMatrix" border size="small" style="margin-bottom: 10px">
                  <el-table-column label="深度 \ 档位" width="110" prop="depthText" />
                  <el-table-column v-for="t in niahTiers" :key="t" :label="t" align="center" min-width="110">
                    <template #default="{ row }">
                      <el-tag v-if="row.cells[t]" size="small" :type="hitType(row.cells[t].hit_rate)" effect="plain">
                        {{ (row.cells[t].hit_rate * 100).toFixed(0) }}%({{ row.cells[t].hits }}/{{ row.cells[t].probes }})
                      </el-tag>
                      <span v-else>-</span>
                    </template>
                  </el-table-column>
                  <el-table-column label="说明" min-width="140">
                    <template #default="{ row }">
                      <span style="font-size: 12px; color: #909399">埋针深度 {{ row.depthText }},每格数字为 命中率(命中/探针)</span>
                    </template>
                  </el-table-column>
                </el-table>
              </template>
            </el-card>
          </template>
          <el-empty v-if="!items.length" description="尚无测试项(任务启动后自动生成)" />
        </el-tab-pane>

        <el-tab-pane label="请求明细" name="samples">
          <el-card shadow="never">
            <div class="filter-bar">
              <el-select v-model="sampleFilter.phase" placeholder="阶段" clearable style="width: 120px" @change="loadSamples">
                <el-option v-for="(n, k) in phaseName" :key="k" :label="n" :value="k" />
              </el-select>
              <el-select v-model="sampleFilter.tier" placeholder="档位" clearable style="width: 100px" @change="loadSamples">
                <el-option v-for="t in ['1K','10K','20K','30K']" :key="t" :label="t" :value="t" />
              </el-select>
              <el-select v-model="sampleFilter.status" placeholder="状态" clearable style="width: 110px" @change="loadSamples">
                <el-option label="成功" value="ok" /><el-option label="超时" value="timeout" />
                <el-option label="错误" value="error" /><el-option label="跳过" value="skipped" />
              </el-select>
              <el-input-number v-model="sampleFilter.run_no" :min="0" placeholder="run" style="width: 110px" @change="loadSamples" />
              <span style="color: #909399; font-size: 12px">run=0 显示全部历史 run</span>
            </div>
            <el-table :data="samples" border size="small" v-loading="samplesLoading">
              <el-table-column type="expand">
                <template #default="{ row }">
                  <div style="padding: 8px 14px">
                    <pre v-if="row.raw_response" class="resp-pre">{{ row.raw_response }}</pre>
                    <span v-else style="color: #909399">无返回内容记录(预热/校准或失败样本)</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column prop="id" label="ID" width="70" />
              <el-table-column label="时间" width="165">
                <template #default="{ row }">{{ fmtTime(row.send_ts) }}</template>
              </el-table-column>
              <el-table-column prop="run_no" label="run" width="60" />
              <el-table-column label="阶段" width="80">
                <template #default="{ row }">{{ phaseName[row.phase] || row.phase }}</template>
              </el-table-column>
              <el-table-column prop="tier" label="档位" width="70" />
              <el-table-column prop="concurrency" label="并发" width="60" />
              <el-table-column label="TTFT(ms)" width="100">
                <template #default="{ row }">{{ row.ttft_ms ? row.ttft_ms.toFixed(0) : '-' }}</template>
              </el-table-column>
              <el-table-column label="总耗时(ms)" width="100">
                <template #default="{ row }">{{ row.total_ms ? row.total_ms.toFixed(0) : '-' }}</template>
              </el-table-column>
              <el-table-column label="prompt" width="90">
                <template #default="{ row }">
                  {{ row.prompt_tokens }}<el-tag v-if="row.tokens_estimated" size="small" type="warning" effect="plain" style="margin-left: 2px">估</el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="completion_tokens" label="completion" width="90" />
              <el-table-column label="速度" width="80">
                <template #default="{ row }">{{ row.speed ? row.speed.toFixed(1) : '-' }}</template>
              </el-table-column>
              <el-table-column label="状态" width="80">
                <template #default="{ row }">
                  <el-tag size="small" :type="{ ok: 'success', timeout: 'danger', error: 'danger', skipped: 'warning' }[row.status]">
                    {{ { ok: '成功', timeout: '超时', error: '错误', skipped: '跳过' }[row.status] }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="标记" min-width="130">
                <template #default="{ row }">
                  <el-tag v-if="row.output_incomplete" size="small" type="warning" effect="plain">输出未满</el-tag>
                  <el-tag v-if="row.length_mismatch" size="small" type="danger" effect="plain">长度未达标</el-tag>
                  <span v-if="row.error" style="font-size: 11px; color: #f56c6c">{{ row.error.slice(0, 40) }}</span>
                </template>
              </el-table-column>
            </el-table>
            <el-pagination style="margin-top: 10px" background layout="total, prev, pager, next"
              :total="sampleTotal" :page-size="50" @current-change="(p) => loadSamples(p)" />
          </el-card>
        </el-tab-pane>

        <el-tab-pane label="运行日志" name="logs">
          <el-card shadow="never">
            <div class="filter-bar">
              <el-button size="small" @click="loadLogs()"><el-icon><Refresh /></el-icon>刷新</el-button>
              <el-button size="small" @click="downloadLog"><el-icon><Download /></el-icon>下载日志</el-button>
              <el-checkbox v-model="logFollow">自动滚动</el-checkbox>
            </div>
            <div ref="logBox" class="log-box">
              <div v-for="l in logs" :key="l.id" :class="['log-line', 'log-' + l.level]">
                <span class="log-ts">{{ fmtTime(l.ts) }}</span> {{ l.message }}
              </div>
              <div v-if="!logs.length" style="color: #606266">暂无日志</div>
            </div>
          </el-card>
        </el-tab-pane>
      </el-tabs>

      <el-dialog v-model="artifactDialog" title="生成产物预览" width="900px" top="4vh" destroy-on-close>
        <div style="display: flex; gap: 8px; margin-bottom: 10px">
          <el-button size="small" @click="openArtifactNew"><el-icon><TopRight /></el-icon>新窗口打开</el-button>
          <span style="color: #909399; font-size: 12px; align-self: center">{{ artifactPath }}</span>
        </div>
        <iframe v-if="artifactDialog" :src="artifactUrl" style="width: 100%; height: 62vh; border: 1px solid #e4e7ed; border-radius: 6px" />
      </el-dialog>

      <!-- V2.23:数据包对话框——服务器物化目录绝对路径+文件清单(无 zip 下载) -->
      <el-dialog v-model="pkgDialog" title="任务数据包" width="720px" top="6vh" destroy-on-close>
        <div v-loading="pkgLoading">
          <template v-if="pkgInfo">
            <el-alert v-if="!pkgInfo.frozen" type="warning" :closable="false"
              title="任务尚未结束,以下为当前已产生的数据(未定型,每次点击实时生成,不缓存)" style="margin-bottom: 12px" />
            <template v-if="pkgInfo.frozen">
              <div style="font-size: 13px; color: #606266; margin-bottom: 6px">本任务全部数据已保存至服务器目录:</div>
              <div style="display: flex; gap: 8px; margin-bottom: 12px">
                <el-input :model-value="pkgInfo.dir" readonly size="small" />
                <el-button size="small" type="primary" @click="copyPkgPath">复制路径</el-button>
              </div>
            </template>
            <div style="font-size: 13px; color: #606266; margin-bottom: 6px">文件清单(共 {{ (pkgInfo.files || []).length }} 个):</div>
            <div style="max-height: 46vh; overflow: auto; border: 1px solid #e4e7ed; border-radius: 6px">
              <table style="width: 100%; border-collapse: collapse; font-size: 12px">
                <tr v-for="f in pkgInfo.files" :key="f.path" style="border-bottom: 1px solid #f2f4f7">
                  <td style="padding: 5px 10px; font-family: monospace">{{ f.path }}</td>
                  <td style="padding: 5px 10px; text-align: right; color: #909399; white-space: nowrap">{{ fmtSizeB(f.size) }}</td>
                </tr>
              </table>
            </div>
          </template>
        </div>
      </el-dialog>

      <el-dialog v-model="editDialog" :title="`编辑任务 ${task.id}`" width="1100px" top="4vh" destroy-on-close>
        <el-alert v-if="editForm && editForm._fromTerminal" type="warning" :closable="false"
          title="该任务此前已执行过:保存后状态重置为「待执行」,重新启动时 run+1,历史数据保留" style="margin-bottom: 12px" />
        <div v-if="editForm" style="display: flex; align-items: center; gap: 10px; margin-bottom: 14px">
          <span style="flex: none; color: #606266; font-size: 14px">项目名称</span>
          <el-input v-model="editForm.project_name" placeholder="项目名称" style="max-width: 360px" />
          <span style="flex: none; color: #606266; font-size: 14px">测试人</span>
          <el-input v-model="editForm.project_tester" placeholder="测试人" style="max-width: 160px" />
          <span style="flex: none; color: #606266; font-size: 14px">项目备注</span>
          <el-input v-model="editForm.project_note" placeholder="项目备注" style="max-width: 360px" />
        </div>
        <HwForm v-if="editForm && project && project.type === 'hardware'" v-model="editForm.hw" />
        <el-divider v-if="editForm && project && project.type === 'hardware'" content-position="left">模型任务配置</el-divider>
        <TaskForm v-if="editDialog && editForm" v-model="editForm" :corpus-versions="corpusVersions" dense />
        <template #footer>
          <el-button @click="editDialog = false">取消</el-button>
          <el-button type="primary" :loading="savingEdit" @click="saveEdit">保存</el-button>
        </template>
      </el-dialog>
    </template>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'
import BackBar from '../components/BackBar.vue'
import Chart from '../components/Chart.vue'
import TaskForm from '../components/TaskForm.vue'
import HwForm from '../components/HwForm.vue'
import { blankHw, fmtITL, fmtNum, fmtTime, fmtTimeShort, fmtTokens, formatContextLen, itemStatus, itemTypeName, normalizeTaskParams, phaseName, taskStatus } from '../utils'

const route = useRoute()
const router = useRouter()
const taskId = route.params.id
const task = ref(null)
const items = ref([])
const summary = ref({})
const project = ref(null)
const runs = ref([])
const selectedRun = ref(0)
const results = ref({ stability: {} })
const quality = ref({ list: [] })
const samples = ref([])
const sampleTotal = ref(0)
const samplesLoading = ref(false)
const sampleFilter = ref({ phase: '', tier: '', status: '', run_no: 0 })
const logs = ref([])
const logFollow = ref(true)
const logBox = ref(null)
const activeTab = ref('results')
const cacheCN = { on: '开', off: '关', unknown: '未知' }
const artifactDialog = ref(false)
const artifactPath = ref('')
const corpusVersions = ref([])
const editDialog = ref(false)
const editForm = ref(null)
const savingEdit = ref(false)
const specFiles = ref([])
const hwInfo = computed(() => {
  try { return JSON.parse(project.value?.hw_json || '{}') } catch { return {} }
})
const fmtSize = (n) => (n > 1 << 20 ? (n / (1 << 20)).toFixed(1) + ' MB' : Math.max(1, Math.round(n / 1024)) + ' KB')
const loadSpecFiles = async () => {
  if (!project.value || project.value.type !== 'hardware') return
  const { data } = await api.get(`/projects/${project.value.id}/files`)
  specFiles.value = data || []
}
const onPickSpec = async (uploadFile) => {
  const fd = new FormData()
  fd.append('file', uploadFile.raw)
  await api.post(`/projects/${project.value.id}/files`, fd, { headers: { 'Content-Type': 'multipart/form-data' } })
  ElMessage.success('规格书已上传')
  loadSpecFiles()
}
const downloadSpec = (f) => window.open(`/api/project-files/${f.id}`, '_blank')
const removeSpec = async (f) => {
  await api.delete(`/project-files/${f.id}`)
  ElMessage.success('已删除')
  loadSpecFiles()
}

// 编辑任务(待执行/失败/已取消/中断);保存后重置待执行,重跑 run+1
const openEdit = () => {
  const t = task.value
  editForm.value = {
    id: t.id,
    project_name: project.value?.name || '',
    project_tester: project.value?.tester || '',
    project_note: project.value?.note || '',
    api_base_url: t.api_base_url, model_name: t.model_name, model_hf_id: t.model_hf_id,
    model_ms_id: t.model_ms_id, quant: t.quant, context_len: t.context_len,
    cache_enabled: t.cache_enabled, cache_note: t.cache_note,
    api_key: '', api_key_set: t.api_key_set, api_key_clear: false,
    corpus_version_id: t.corpus_version_id || null,
    params: JSON.parse(t.params_json),
    hw: (() => { try { return { ...blankHw(), ...JSON.parse(project.value?.hw_json || '{}') } } catch { return blankHw() } })(),
    _fromTerminal: ['failed', 'cancelled', 'interrupted'].includes(t.status),
  }
  editDialog.value = true
}
const saveEdit = async () => {
  const t = editForm.value
  if (!t.api_base_url || !t.model_name) { ElMessage.warning('API 地址与模型名称必填'); return }
  if (!t.project_name || !t.project_name.trim()) { ElMessage.warning('项目名称不能为空'); return }
  savingEdit.value = true
  try {
    // 先存任务配置(可能校验失败),成功后再改项目信息,避免状态不一致
    await api.put(`/tasks/${t.id}`, {
      api_base_url: t.api_base_url, model_name: t.model_name, model_hf_id: t.model_hf_id,
      model_ms_id: t.model_ms_id, quant: t.quant, context_len: t.context_len,
      cache_enabled: t.cache_enabled, cache_note: t.cache_note,
      api_key: t.api_key, api_key_clear: t.api_key_clear,
      corpus_version_id: t.corpus_version_id || undefined,
      params: normalizeTaskParams(t.params),
    })
    // 项目名称/测试人/备注任一变更时同步更新项目;硬件项目的硬件信息快照随保存一并更新(V3.0.1)
    const p = project.value
    const metaChanged = p && (t.project_name.trim() !== p.name || (t.project_tester || '') !== (p.tester || '') || (t.project_note || '') !== (p.note || ''))
    if (p && (metaChanged || p.type === 'hardware')) {
      const body = { name: t.project_name.trim(), tester: t.project_tester || '', note: t.project_note || '' }
      if (p.type === 'hardware') body.hw = t.hw
      await api.put(`/projects/${p.id}`, body)
    }
    ElMessage.success('任务配置已保存')
    editDialog.value = false
    loadSummary()
  } finally {
    savingEdit.value = false
  }
}
// 已完成任务:复制为新项目+新任务(旧数据不动),跳转到新任务
const copyTask = async () => {
  const { data } = await api.post(`/tasks/${taskId}/clone`)
  ElMessage.success('已复制为新任务(待执行)')
  if (data && data.id) router.push(`/tasks/${data.id}`)
}
const catName = (c) => ({ common: '常识', reasoning: '推理', instruction: '指令', code: '代码',
  creative: '创作', summary: '摘要', needle: '找回', consistency: '一致性', doc: '文档' }[c] || c || '-')
const artifactUrl = computed(() => `/api/artifacts/${artifactPath.value}/index.html`)
const openArtifact = (row) => {
  artifactPath.value = row.artifact_path
  artifactDialog.value = true
}
const openArtifactNew = () => window.open(artifactUrl.value, '_blank')
// 文档类产物(xlsx/pptx/docx/pdf)直接下载渲染好的文件
const downloadArtifact = (row) => {
  window.open(`/api/artifacts/${row.artifact_path}/document.${row.artifact_type}`, '_blank')
}
let es = null
let pollTimer = null
let watchTimer = null
let lastLogId = 0

const qualityType = computed(() => {
  const q = summary.value.quality
  return q === '正常' ? 'success' : q === '疑似异常' ? 'warning' : q === '异常' ? 'danger' : 'info'
})

// 摘要卡按实际执行的测试项自适应(纯能力任务不显示性能空卡)
const hasLadder = computed(() => (results.value.ladder || []).some((r) => !r.skipped && r.samples > 0))
const hasConcurrency = computed(() => (results.value.concurrency || []).some((r) => !r.circuit_trip && r.throughput > 0))
const hasStability = computed(() => ((results.value.stability || {}).windows || []).length > 0)
// V2.20:稳定性三要素(档位/并发/时长)醒目标注
const stabParamsText = computed(() => {
  try {
    const ex = JSON.parse(task.value?.params_json || '{}').exec || {}
    if (!ex.stability_minutes) return ''
    return `压测参数:输入档 ${ex.stability_tier || '10K'} · 每波并发 ${ex.stability_concurrency || 1} · 时长 ${ex.stability_minutes} 分钟`
  } catch { return '' }
})
const hasQuality = computed(() => (quality.value.list || []).length > 0)
const capOverall = ref(null)
const loadCapabilityScore = async () => {
  const { data } = await api.get('/compare/capability', { params: { ids: taskId } })
  const row = (data.rows || [])[0]
  capOverall.value = row ? row.overall : null
}

const loadSummary = async () => {
  const { data } = await api.get(`/tasks/${taskId}/summary`)
  task.value = data.task
  items.value = data.items || []
  summary.value = data.summary || {}
  project.value = data.project || null
  runs.value = data.runs || [data.task.run_no]
  if (!selectedRun.value || selectedRun.value > data.task.run_no) selectedRun.value = data.task.run_no
}
const onRunChange = () => {
  loadResults()
  loadQuality()
}
const loadResults = async () => {
  const { data } = await api.get(`/tasks/${taskId}/results`, { params: { run_no: selectedRun.value } })
  results.value = data
}
const loadQuality = async () => {
  const { data } = await api.get(`/tasks/${taskId}/quality`, { params: { run_no: selectedRun.value } })
  for (const row of data.list || []) {
    row._flag = row.manual_flag || ''
    row._note = row.manual_note || ''
    row._dirty = false
  }
  quality.value = data
}
const loadSamples = async (page = 1) => {
  samplesLoading.value = true
  try {
    const params = { page, page_size: 50 }
    for (const k of ['phase', 'tier', 'status']) if (sampleFilter.value[k]) params[k] = sampleFilter.value[k]
    if (sampleFilter.value.run_no > 0) params.run_no = sampleFilter.value.run_no
    const { data } = await api.get(`/tasks/${taskId}/samples`, { params })
    samples.value = data.list || []
    sampleTotal.value = data.total
  } finally {
    samplesLoading.value = false
  }
}
const loadLogs = async () => {
  const { data } = await api.get(`/tasks/${taskId}/logs`, { params: { after_id: lastLogId } })
  const list = data.list || []
  if (list.length) {
    lastLogId = list[list.length - 1].id
    logs.value.push(...list)
    if (logs.value.length > 3000) logs.value = logs.value.slice(-3000)
    if (logFollow.value) nextTick(() => { if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight })
  }
}
const downloadLog = () => window.open(`/api/tasks/${taskId}/logs?download=1`, '_blank')
// V2.23:数据包物化为服务器真实目录,对话框显示绝对路径+文件清单(无 zip 下载)
const pkgDialog = ref(false)
const pkgInfo = ref(null)
const pkgLoading = ref(false)
const openPackage = async () => {
  pkgLoading.value = true
  pkgDialog.value = true
  try {
    const { data } = await api.get(`/tasks/${taskId}/package-info`)
    pkgInfo.value = data
  } finally {
    pkgLoading.value = false
  }
}
const copyPkgPath = async () => {
  try {
    await navigator.clipboard.writeText(pkgInfo.value.dir)
    ElMessage.success('路径已复制')
  } catch {
    ElMessage.warning('复制失败,请手动选择复制')
  }
}
const fmtSizeB = (n) => (n > 1 << 20 ? (n / (1 << 20)).toFixed(1) + ' MB' : Math.max(1, Math.round(n / 1024)) + ' KB')
const fmtInt = (n) => (n ? Number(n).toLocaleString() : '0')
const cancel = async () => { await api.post(`/tasks/${taskId}/cancel`); loadSummary() }
const restart = async () => { await api.post(`/tasks/${taskId}/start`); ElMessage.success('已入队重跑'); loadAll() }
const resume = async () => { await api.post(`/tasks/${taskId}/resume`); ElMessage.success('继续跑已开始'); loadAll() }
const pauseTask = async () => {
  await ElMessageBox.confirm('暂停后任务中止当前请求并释放执行名额,点「继续跑」从断点续跑。确定暂停?', '暂停任务', { type: 'warning' })
  await api.post(`/tasks/${taskId}/pause`)
  ElMessage.success('已请求暂停')
  loadSummary()
}
const skipItem = async () => {
  await ElMessageBox.confirm('当前测试项将标记为「已跳过」并立即进入下一项;已采集数据保留,任务结束后可对该项补测。确定跳过?', '跳过当前项', { type: 'warning' })
  await api.post(`/tasks/${taskId}/skip-item`)
  ElMessage.success('已请求跳过当前项')
  loadSummary()
}
// V2.19 测试项级控制:跳过可点 = 该项执行中(中止本项) 或 任务已暂停且该项未完成(标记跳过)
const canSkipItem = (item) =>
  item.status === 'running' ||
  (task.value?.status === 'paused' && ['pending', 'interrupted', 'cancelled', 'failed'].includes(item.status))
// V2.20 继续可点 = 任务暂停且该项中断/未完成(断点续跑) 或 该项已跳过(重跑该项)
const canContinueItem = (item) =>
  item.status === 'skipped' ||
  (task.value?.status === 'paused' && ['interrupted', 'cancelled', 'pending'].includes(item.status))
const continueItem = async (item) => {
  if (item.status === 'skipped') {
    // 跳过的项:继续 = 重跑该项
    await rerunItem(item)
    return
  }
  // 中断的项:继续 = 任务断点续跑
  await resume()
}
const skipItemById = async (item) => {
  await ElMessageBox.confirm(`「${itemTypeName[item.type]}」将标记为「已跳过」,之后可点「补测」重新执行。确定跳过?`, '跳过测试项', { type: 'warning' })
  await api.post(`/tasks/${taskId}/items/${item.id}/skip`)
  ElMessage.success(`「${itemTypeName[item.type]}」已跳过`)
  loadAll()
}
const rerunItem = async (item) => {
  await api.post(`/tasks/${taskId}/items/${item.id}/rerun`)
  ElMessage.success(`「${itemTypeName[item.type]}」重跑已开始`)
}
// A5:显式确认标注流
const saveQuality = async (row) => {
  if (!row._flag) { ElMessage.warning('请先选择 正常/异常'); return }
  await api.put(`/quality-answers/${row.id}`, { manual_flag: row._flag, manual_note: row._note })
  row.manual_flag = row._flag
  row.manual_note = row._note
  row._dirty = false
  ElMessage.success('标注已保存')
  loadSummary()
}
const clearQuality = async (row) => {
  await api.put(`/quality-answers/${row.id}`, { manual_flag: '', manual_note: '' })
  row.manual_flag = ''
  row.manual_note = ''
  row._flag = ''
  row._note = ''
  row._dirty = false
  ElMessage.success('已恢复为按自动初筛')
  loadSummary()
}

// ---- 图表(F-7.2 六类) ----
const tierIdx = (t) => parseFloat(t) || 999 // V2.5:支持小数档(2.5K)
const ladderRows = computed(() => (results.value.ladder || []).filter((r) => !r.skipped).sort((a, b) => tierIdx(a.tier) - tierIdx(b.tier)))
const baseChart = (title) => ({
  title: { text: title, textStyle: { fontSize: 13 } },
  tooltip: { trigger: 'axis' },
  grid: { left: 60, right: 30, top: 55, bottom: 30 },
})
// V2.10 分布统计列(平均/P50/P95/P99/最短/最长)
const statCols = [
  { k: 'avg', label: '平均', w: 95 }, { k: 'p50', label: 'P50', w: 85 }, { k: 'p95', label: 'P95', w: 85 },
  { k: 'p99', label: 'P99', w: 85 }, { k: 'min', label: '最短', w: 85 }, { k: 'max', label: '最长', w: 85 },
]
const ladderSpeedChart = computed(() => ({
  ...baseChart('阶梯 - 输出速度'),
  xAxis: { type: 'category', data: ladderRows.value.map((r) => r.tier) },
  yAxis: { type: 'value', name: 'tokens/s' },
  series: [{ type: 'line', smooth: true, data: ladderRows.value.map((r) => +r.speed?.avg?.toFixed(2)), itemStyle: { color: '#409EFF' } }],
}))
const ladderTtftChart = computed(() => ({
  ...baseChart('阶梯 - TTFT'),
  xAxis: { type: 'category', data: ladderRows.value.map((r) => r.tier) },
  yAxis: { type: 'value', name: 'ms' },
  series: [{ type: 'line', smooth: true, data: ladderRows.value.map((r) => +r.ttft?.avg?.toFixed(1)), itemStyle: { color: '#E6A23C' } }],
}))
const concurrencyChart = computed(() => {
  // V2.15:多输入档时每档一条线;横轴并发数(全档位并集)
  const all = results.value.concurrency || []
  const tiers = [...new Set(all.map((r) => r.tier || '1K'))]
  const levels = [...new Set(all.filter((r) => !r.circuit_trip).map((r) => r.level))].sort((a, b) => a - b)
  const mkSeries = (pick) => tiers.map((t, i) => ({
    name: t, type: 'line', smooth: true,
    data: levels.map((l) => {
      const r = all.find((x) => x.level === l && (x.tier || '1K') === t && !x.circuit_trip)
      const v = r ? pick(r) : null
      return v != null && !isNaN(v) ? v : null
    }),
  }))
  return { levels, mkSeries }
})
const concurrencyChartData = computed(() => {
  const { levels, mkSeries } = concurrencyChart.value
  return {
    ...baseChart('并发 - 平均每请求速度 TPS(decode 口径,不含 TTFT;每线一输入档)'),
    legend: { bottom: 0, type: 'scroll', textStyle: { fontSize: 11 } },
    grid: { left: 60, right: 30, top: 55, bottom: 55 },
    xAxis: { type: 'category', data: levels.map((l) => 'C' + l) },
    yAxis: { type: 'value', name: 'tokens/s/请求' },
    series: mkSeries((r) => +r.speed?.avg?.toFixed(2)),
  }
})
// 并发 - 首字延迟(平均,V2.10:P99 线撤掉,明细进表格)
const concurrencyTtftChart = computed(() => {
  const { levels, mkSeries } = concurrencyChart.value
  return {
    ...baseChart('并发 - 首字延迟 TTFT(每线一输入档)'),
    legend: { bottom: 0, type: 'scroll', textStyle: { fontSize: 11 } },
    grid: { left: 60, right: 30, top: 55, bottom: 55 },
    xAxis: { type: 'category', data: levels.map((l) => 'C' + l) },
    yAxis: { type: 'value', name: 'ms' },
    series: mkSeries((r) => +r.ttft?.avg?.toFixed(1)),
  }
})
const cacheChart = computed(() => {
const rows = results.value.cache || []
  return {
    ...baseChart('缓存 - 各档位命中率'),
    xAxis: { type: 'category', data: rows.map((r) => r.tier) },
    yAxis: { type: 'value', name: '命中率%', min: 0, max: 100 },
    series: [{ type: 'line', smooth: true, symbolSize: 8,
      data: rows.map((r) => +(r.hit_rate * 100).toFixed(1)),
      label: { show: true, position: 'top', formatter: '{c}%' },
      itemStyle: { color: '#67C23A' } }],
  }
})

// ---- NIAH 矩阵(V2.5 legacy / V2.13 verify):行=埋针深度,列=输入长度点位,格=命中率 ----
const niahMode = computed(() => (results.value.niah || [])[0]?.mode || 'legacy')
const niahIsVerify = computed(() => niahMode.value === 'verify')
const niahDepthKey = (c) => (niahIsVerify.value ? c.depth_pct : c.depth)
const niahTiers = computed(() => [...new Set((results.value.niah || []).map((c) => c.tier))]
  .sort((a, b) => tierIdx(a) - tierIdx(b)))
const niahDepths = computed(() => [...new Set((results.value.niah || []).map(niahDepthKey))].sort((a, b) => a - b))
const niahMatrix = computed(() => niahDepths.value.map((d) => {
  const cells = {}
  for (const c of (results.value.niah || [])) if (niahDepthKey(c) === d) cells[c.tier] = c
  return { depthText: d + '%', cells }
}))
const hitType = (r) => (r >= 0.999 ? 'success' : r >= 0.5 ? 'warning' : 'danger')

// V2.15:并发合并表输入档列跨行合并(同档只显示一次)
const concSpan = ({ row, column, rowIndex, columnIndex }) => {
  if (columnIndex !== 0) return
  const rows = results.value.concurrency || []
  const tier = row.tier || '1K'
  if (rowIndex > 0 && (rows[rowIndex - 1].tier || '1K') === tier) {
    return { rowspan: 0, colspan: 0 }
  }
  let span = 0
  for (let i = rowIndex; i < rows.length && (rows[i].tier || '1K') === tier; i++) span++
  return { rowspan: span, colspan: 1 }
}
// ---- NIAH 扫描视图(V2.8 scan / V2.13 discover):点位=输入长度 ----
const niahIsScan = computed(() => (niahMode.value === 'scan' || niahMode.value === 'discover') && (results.value.niah || []).length > 0)
const niahScanRows = computed(() => [...(results.value.niah || [])].sort((a, b) => (a.tokens || a.pct) - (b.tokens || b.pct)))
const niahScanChart = computed(() => {
  const rows = niahScanRows.value
  const eff = results.value.niah_effective
  const unit = niahMode.value === 'discover' ? 'K' : '%'
  const labels = rows.map((r) => `${r.tier}\n${r.pct}${unit}`)
  const markLine = eff && !eff.at_ceiling
    ? { symbol: 'none', lineStyle: { color: '#f56c6c', type: 'dashed', width: 2 },
        label: { show: true, formatter: '实测边界 ' + fmtTokens(eff.tokens), position: 'insideEndTop', color: '#f56c6c' },
        data: [{ xAxis: rows.findIndex((r) => r.tokens === eff.tokens) }] }
    : undefined
  return {
    ...baseChart('NIAH 有效上下文扫描(横轴=输入长度,纵轴=密钥找回率)'),
    grid: { left: 60, right: 30, top: 50, bottom: 60 },
    tooltip: {
      trigger: 'axis',
      formatter: (ps) => {
        const r = rows[ps[0].dataIndex]
        return `输入长度 ${r.tier} · 埋针深度 ${r.depth_pct}%<br/>找回率 ${(r.hit_rate * 100).toFixed(0)}%(${r.hits}/${r.probes} 次)`
          + (r.ttft_median > 0 ? `<br/>TTFT 中位 ${r.ttft_median.toFixed(0)} ms` : '')
          + (r.errors > 0 ? `<br/>错误/截断 ${r.errors} 次(不计入)` : '')
      },
    },
    xAxis: { type: 'category', data: labels, axisLabel: { interval: 0, fontSize: 10, lineHeight: 14 } },
    yAxis: { type: 'value', name: '找回率%', max: 100, min: 0 },
    series: [{
      name: '找回率', type: 'bar', barMaxWidth: 42,
      data: rows.map((r) => ({
        value: +(r.hit_rate * 100).toFixed(1),
        itemStyle: { color: r.hit_rate >= 0.999 ? '#16a34a' : r.hit_rate >= 0.5 ? '#d97706' : '#dc2626' },
      })),
      markLine,
    }],
  }
})
// V2.22:NIAH 结果改热力图(X=长度点位,Y=埋针深度,绿=命中/红=丢失,整片红=有效上下文边界)
const niahHeatmap = computed(() => {
  const rows = results.value.niah || []
  const lens = niahTiers.value
  const depths = niahDepths.value
  const data = []
  depths.forEach((d, y) => lens.forEach((t, x) => {
    const c = rows.find((v) => v.tier === t && niahDepthKey(v) === d)
    data.push([x, y, c ? +(c.hit_rate * 100).toFixed(0) : '-'])
  }))
  return {
    tooltip: {
      formatter: (p) => {
        const c = rows.find((v) => v.tier === lens[p.value[0]] && niahDepthKey(v) === depths[p.value[1]])
        if (!c) return '未探测'
        return `${c.tier} · 深度 ${depths[p.value[1]]}%<br/>命中率 ${p.value[2]}%(${c.hits}/${c.probes} 次)`
          + (c.ttft_median > 0 ? `<br/>TTFT 中位 ${c.ttft_median.toFixed(0)} ms` : '')
      },
    },
    grid: { left: 70, right: 80, top: 40, bottom: 44 },
    xAxis: { type: 'category', data: lens, name: '上下文长度' },
    yAxis: { type: 'category', data: depths.map((d) => d + '%'), name: '埋针深度' },
    visualMap: {
      min: 0, max: 100, calculable: false, orient: 'vertical', right: 0, top: 'center',
      inRange: { color: ['#dc2626', '#f59e0b', '#16a34a'] }, text: ['命中', '丢失'], textStyle: { fontSize: 10 },
    },
    series: [{ type: 'heatmap', data, label: { show: true, fontSize: 10, formatter: (p) => p.value[2] } }],
    title: { text: 'NIAH 命中率热力图', left: 'center', textStyle: { fontSize: 13 } },
  }
})
const niahHeatHeight = computed(() => Math.max(220, 90 + niahDepths.value.length * 46))
// V2.10:稳定性改为逐次调用散点图(X=调用序号),一图速度一图首字延迟
const stabPoints = computed(() => ((results.value.stability?.points) || []).filter((p) => p.ok))
const stabSpeedScatter = computed(() => ({
  ...baseChart('稳定性 - 每次调用的输出速度'),
  xAxis: { type: 'value', name: '调用序号', min: 1 },
  yAxis: { type: 'value', name: 'tokens/s', scale: true },
  series: [{ type: 'scatter', symbolSize: 6, itemStyle: { color: '#409EFF', opacity: 0.7 },
    data: stabPoints.value.map((p) => [p.i, +p.speed.toFixed(2)]) }],
}))
const stabTtftScatter = computed(() => ({
  ...baseChart('稳定性 - 每次调用的首字延迟 TTFT'),
  xAxis: { type: 'value', name: '调用序号', min: 1 },
  yAxis: { type: 'value', name: 'ms', scale: true },
  series: [{ type: 'scatter', symbolSize: 6, itemStyle: { color: '#E6A23C', opacity: 0.7 },
    data: stabPoints.value.map((p) => [p.i, +p.ttft.toFixed(1)]) }],
}))

// ---- SSE 实时进度,断线退化 2s 轮询(§7.1) ----
const startSSE = () => {
  stopSSE()
  try {
    es = new EventSource(`/api/tasks/${taskId}/events`)
    es.addEventListener('progress', () => loadSummary())
    es.addEventListener('status', () => { loadSummary(); loadResults(); loadQuality() })
    es.addEventListener('item', () => { loadSummary(); loadResults() })
    es.addEventListener('log', () => loadLogs())
    es.onerror = () => {
      stopSSE()
      if (['running', 'queued'].includes(task.value?.status)) {
        pollTimer = setInterval(() => { loadSummary(); loadLogs() }, 2000)
      }
    }
  } catch {
    if (['running', 'queued'].includes(task.value?.status)) {
      pollTimer = setInterval(() => { loadSummary(); loadLogs() }, 2000)
    }
  }
}
const stopSSE = () => {
  if (es) { es.close(); es = null }
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}

const loadAll = async () => {
  await Promise.all([loadSummary(), loadResults(), loadQuality(), loadSamples(), loadLogs(), loadCapabilityScore()])
  loadSpecFiles()
}

onMounted(async () => {
  await loadAll()
  const { data: cv } = await api.get('/corpus-versions')
  corpusVersions.value = cv || []
  if (['running', 'queued'].includes(task.value?.status)) startSSE()
  // 兜底巡检:任务执行中确保 SSE/轮询挂着,完成后拉最终结果
  watchTimer = setInterval(async () => {
    const prev = task.value?.status
    await loadSummary()
    const cur = task.value?.status
    if (['running', 'queued'].includes(cur)) {
      if (!es && !pollTimer) startSSE()
      if (!es) loadLogs()
    } else if (['running', 'queued'].includes(prev)) {
      stopSSE()
      loadResults(); loadQuality(); loadSamples(); loadCapabilityScore()
    }
  }, 4000)
})
onUnmounted(() => { stopSSE(); clearInterval(watchTimer) })
</script>

<style scoped>
.log-box { background: #1f2d3d; color: #c0c4cc; border-radius: 6px; padding: 10px 12px;
  height: 420px; overflow-y: auto; font-family: Consolas, 'Courier New', monospace; font-size: 12.5px; }
.log-line { line-height: 1.7; white-space: pre-wrap; word-break: break-all; }
.log-ts { color: #5f738c; margin-right: 6px; }
.log-warn { color: #e6a23c; }
.log-error { color: #f56c6c; }
.resp-pre { white-space: pre-wrap; word-break: break-all; font-size: 12px; margin: 0;
  max-height: 300px; overflow-y: auto; background: #f8f9fa; padding: 8px; border-radius: 6px; }
.spec-row { display: flex; align-items: center; gap: 8px; padding: 7px 10px; border: 1px solid #e4e7ed;
  border-radius: 6px; margin-top: 8px; background: #fafafa; }
.spec-name { font-size: 13px; color: #303133; word-break: break-all; }
.spec-meta { flex: 1; font-size: 12px; color: #909399; }
</style>
