<script setup lang="ts">
import { computed } from 'vue';
import type { WindowImpactAssessment } from '../../types/domain';
import { formatDate, WINDOW_STATUS_LABELS, PLAN_IMPACT_LABELS, CLEARANCE_IMPACT_LABELS, impactTone } from '../../utils/format';
import StatusBadge from './StatusBadge.vue';

const props = defineProps<{ modelValue: boolean; assessment: WindowImpactAssessment | null; loading: boolean }>();
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>();

const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
});

const windowLabel = (status: string) => WINDOW_STATUS_LABELS[status] || status;
</script>

<template>
  <el-dialog v-model="visible" title="窗口影响评估" width="860px" destroy-on-close>
    <div v-if="assessment" class="impact-body">
      <section class="impact-summary">
        <header>
          <div>
            <span class="eyebrow">IMPACT ASSESSMENT</span>
            <strong>{{ assessment.window.code }} · {{ assessment.window.name }}</strong>
            <small>作业区：{{ assessment.window.facility }} ｜ 关联事项：{{ assessment.window.relatedCode || '未关联' }} ｜ 窗口 v{{ assessment.window.version }}（{{ windowLabel(assessment.window.status) }}）</small>
          </div>
          <div :class="`impact-decision impact-decision--${assessment.decision}`">
            <strong>{{ assessment.decision === 'continue' ? '现场继续' : '现场暂停' }}</strong>
          </div>
        </header>
        <p class="decision-reason">{{ assessment.decisionReason }}</p>
        <footer>
          <span>已过期许可 <b>{{ assessment.expiredClearances }}</b></span>
          <span>待重新确认 <b>{{ assessment.reboundClearances }}</b></span>
          <span>受阻许可 <b>{{ assessment.blockedClearances }}</b></span>
          <span>当前版本有效 <b>{{ assessment.readyClearances }}</b></span>
          <small>评估时间：{{ formatDate(assessment.assessedAt) }}</small>
        </footer>
      </section>

      <h4>同作业区 / 关联事项下的系泊方案（{{ assessment.plans.length }}）</h4>
      <el-table :data="assessment.plans" size="small" empty-text="该作业区与关联事项下暂无系泊方案">
        <el-table-column prop="code" label="方案编码" width="120"/>
        <el-table-column label="方案名称" min-width="160">
          <template #default="{ row }"><strong>{{ row.name }}</strong><small>{{ row.owner }}</small></template>
        </el-table-column>
        <el-table-column label="方案状态" width="110">
          <template #default="{ row }"><StatusBadge :status="row.status"/></template>
        </el-table-column>
        <el-table-column label="影响结论" width="170">
          <template #default="{ row }">
            <span :class="`status status--${impactTone(row.impact)}`">{{ PLAN_IMPACT_LABELS[row.impact] || row.impact }}</span>
          </template>
        </el-table-column>
      </el-table>

      <h4>同作业区 / 关联事项下的安全许可（{{ assessment.clearances.length }}）</h4>
      <el-table :data="assessment.clearances" size="small" empty-text="该作业区与关联事项下暂无安全许可">
        <el-table-column prop="code" label="许可编码" width="120"/>
        <el-table-column label="许可状态" width="100">
          <template #default="{ row }"><StatusBadge :status="row.status"/></template>
        </el-table-column>
        <el-table-column label="绑定窗口版本" width="130">
          <template #default="{ row }">
            {{ row.windowCode || '按关联事项匹配' }} · v{{ row.windowVersion || 1 }}
            <el-tag v-if="row.rebindRequired" type="warning" size="small" effect="plain">待重新确认</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="提交人 / 复核人" width="160">
          <template #default="{ row }">
            <small>{{ row.submittedBy || '待提交' }} / {{ row.confirmedBy || '待复核' }}</small>
          </template>
        </el-table-column>
        <el-table-column label="影响结论" min-width="180">
          <template #default="{ row }">
            <span :class="`status status--${impactTone(row.impact)}`">{{ CLEARANCE_IMPACT_LABELS[row.impact] || row.impact }}</span>
          </template>
        </el-table-column>
      </el-table>
    </div>
    <div v-else-if="loading" class="impact-loading">正在加载影响评估…</div>
    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.impact-body { display: grid; gap: 10px; }
.impact-summary { border: 1px solid #dbe4e8; background: #f7fafb; padding: 14px 16px; }
.impact-summary header { display: flex; justify-content: space-between; gap: 16px; align-items: flex-start; }
.impact-summary strong { display: block; margin: 4px 0; font-size: 16px; }
.impact-summary small { color: #687b88; }
.impact-decision { padding: 8px 16px; border-radius: 4px; font-size: 15px; white-space: nowrap; }
.impact-decision--continue { background: #dff3ec; color: #176c55; }
.impact-decision--suspend { background: #fde4e6; color: #9d2c37; }
.decision-reason { margin: 12px 0; color: #33485a; font-weight: 600; }
.impact-summary footer { display: flex; flex-wrap: wrap; gap: 18px; color: #576b78; font-size: 13px; }
.impact-summary footer b { color: #172d3f; }
.impact-summary footer small { margin-left: auto; color: #82919a; }
h4 { margin: 8px 0 0; }
.impact-loading { padding: 40px; text-align: center; color: #176c55; }
</style>
