<script setup lang="ts">
import { computed } from 'vue';
import type { WindowImpact } from '../../types/domain';
import StatusBadge from './StatusBadge.vue';
import { formatDate } from '../../utils/format';

const props = defineProps<{ impact: WindowImpact | null; loading: boolean }>();

const decisionTitle = computed(() => props.impact?.decision === 'continue' ? '现场结论：可继续作业' : '现场结论：立即暂停');
const decisionType = computed(() => (props.impact?.decision === 'continue' ? 'success' : 'error') as 'success' | 'error');
</script>

<template>
  <div v-loading="loading" class="impact-panel">
    <template v-if="impact">
      <el-alert :title="decisionTitle" :description="impact.decisionReason" :type="decisionType" show-icon :closable="false"/>
      <section class="impact-meta">
        <article>
          <span>窗口版本</span><strong>v{{ impact.window.version }}</strong>
          <small><StatusBadge :status="impact.window.status"/></small>
        </article>
        <article>
          <span>关联方案</span><strong>{{ impact.planCount }}</strong>
          <small>同作业区 / 同关联事项</small>
        </article>
        <article>
          <span>关联许可</span><strong>{{ impact.clearanceCount }}</strong>
          <small>已过期 {{ impact.expiredCount }} · 版本不符 {{ impact.staleCount }}</small>
        </article>
        <article>
          <span>作业区 / 关联事项</span><strong>{{ impact.window.facility }}</strong>
          <small>{{ impact.window.relatedCode || '未设置关联事项' }} · 更新于 {{ formatDate(impact.window.updatedAt) }}</small>
        </article>
      </section>

      <h4>受影响的系泊方案</h4>
      <el-table :data="impact.plans" size="small" empty-text="同作业区、同一关联事项下暂无方案">
        <el-table-column prop="code" label="编码" width="110"/>
        <el-table-column label="方案" min-width="150">
          <template #default="{ row }"><strong>{{ row.name }}</strong><small>{{ row.owner }}</small></template>
        </el-table-column>
        <el-table-column label="状态" width="105"><template #default="{ row }"><StatusBadge :status="row.status"/></template></el-table-column>
        <el-table-column prop="riskLevel" label="风险" width="80"/>
        <el-table-column prop="recommendation" label="处置建议" min-width="220"/>
      </el-table>

      <h4>受影响的安全许可</h4>
      <el-table :data="impact.clearances" size="small" empty-text="同作业区、同一关联事项下暂无许可">
        <el-table-column prop="code" label="编码" width="110"/>
        <el-table-column label="许可" min-width="150">
          <template #default="{ row }">
            <strong>{{ row.name }}</strong>
            <small>提交：{{ row.submittedBy || '待提交' }} · 复核：{{ row.confirmedBy || '待复核' }}</small>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <StatusBadge :status="row.status"/>
            <el-tag v-if="row.staleVersion" size="small" type="danger" effect="plain" class="stale-tag">版本不符</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="窗口版本" width="90">
          <template #default="{ row }"><span :class="{ 'stale-version': row.staleVersion }">v{{ row.windowVersion }}</span></template>
        </el-table-column>
        <el-table-column prop="recommendation" label="处置建议" min-width="240"/>
      </el-table>
      <p class="impact-note">
        窗口转为受限或过期时，已放行许可同步过期；待复核许可保留提交人并换绑新窗口版本，旧版本提交不能直接放行，须由独立复核人按当前安全窗口版本重新确认。
      </p>
    </template>
    <el-empty v-else-if="!loading" description="暂无影响评估数据"/>
  </div>
</template>

<style scoped>
.impact-panel { display: grid; gap: 14px; }
.impact-meta { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
.impact-meta article { border: 1px solid #dbe4e8; border-left: 3px solid #2a9d78; background: #f7fafb; padding: 10px 12px; min-width: 0; }
.impact-meta span { display: block; color: #6f818d; font-size: 12px; }
.impact-meta strong { display: block; font-size: 18px; margin: 4px 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.impact-meta small { color: #7d8d97; font-size: 12px; }
h4 { margin: 6px 0 0; }
.stale-tag { margin-left: 6px; }
.stale-version { color: #c84855; font-weight: 700; }
.impact-note { color: #667886; font-size: 12px; margin: 0; background: #f3f6f8; border-left: 3px solid #52c6aa; padding: 8px 10px; }
@media (max-width: 900px) { .impact-meta { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
