<script setup lang="ts">
import { computed } from 'vue';
import type { DomainRecord } from '../../types/domain';
import { useAuth } from '../../hooks/useAuth';
import StatusBadge from './StatusBadge.vue';

const props = defineProps<{ records: DomainRecord[]; mode: 'window' | 'clearance' }>();
const emit = defineEmits<{ confirm: [item: DomainRecord] }>();
const { session } = useAuth();
const roleRank: Record<string, number> = { viewer: 1, operator: 2, reviewer: 3, admin: 4 };
const canSubmit = computed(() => (roleRank[session.value?.role || ''] || 0) >= roleRank.operator);
const canReview = computed(() => (roleRank[session.value?.role || ''] || 0) >= roleRank.reviewer);
const displayedRecords = computed(() => {
  const records = [...props.records];
  if (props.mode === 'clearance') {
    records.sort((left, right) => Number(right.status === 'pending') - Number(left.status === 'pending'));
  }
  return records.slice(0, 3);
});

// The release must target the window's current version. After a window rebind
// the pinned submission version is stale, so the action re-confirms against the
// current version rather than silently replaying the old one.
function effectiveWindowVersion(item: DomainRecord): number {
  return item.windowCurrentVersion || item.windowVersion || 1;
}

function windowUnsafe(item: DomainRecord): boolean {
  return !!item.windowLinked && item.windowStatus !== 'safe';
}

function versionDrift(item: DomainRecord): boolean {
  return !!item.windowLinked && !!item.windowCurrentVersion && item.windowCurrentVersion !== (item.windowVersion || 1);
}

function windowLabel(item: DomainRecord): string {
  if (!item.windowLinked) return '关联窗口未找到';
  const versionText = `v${item.windowCurrentVersion || item.windowVersion || 1}`;
  return item.windowStatus ? `窗口 ${item.windowStatus} · ${versionText}` : versionText;
}

function canAct(item: DomainRecord): boolean {
  if (props.mode !== 'clearance' || item.status !== 'pending') return false;
  if (windowUnsafe(item)) return false;
  if (!item.submittedBy) return canSubmit.value;
  return canReview.value && item.submittedBy !== session.value?.username;
}

function actionLabel(item: DomainRecord): string {
  if (!item.submittedBy) return '提交安全确认';
  return versionDrift(item) ? '按新窗口版本重新确认并放行' : '复核并放行';
}

function blockedReason(item: DomainRecord): string {
  if (item.status !== 'pending') return '';
  if (windowUnsafe(item)) return '窗口不安全（受限/过期/未确认安全），禁止放行，待窗口恢复安全后重新确认';
  if (item.submittedBy) {
    if (item.submittedBy === session.value?.username) return '等待其他复核员独立确认';
    if (!canReview.value) return '需 reviewer/admin 完成第二人确认';
  }
  return '';
}
</script>

<template>
  <section class="clearance-panel" aria-label="安全许可协同面板">
    <header>
      <div><span class="eyebrow">TWO-PERSON SAFETY</span><strong>{{ mode === 'window' ? '窗口许可依据' : '双人安全确认' }}</strong></div>
      <small>{{ mode === 'window' ? '窗口版本将随许可审计固化' : '放行前校验窗口安全与版本一致，不安全或版本变化须重新确认' }}</small>
    </header>
    <div class="clearance-grid">
      <article v-for="item in displayedRecords" :key="item.id">
        <div class="clearance-title"><strong>{{ item.code }}</strong><StatusBadge :status="item.status"/></div>
        <p>{{ item.name }}</p>
        <dl>
          <dt>窗口版本</dt>
          <dd>
            v{{ mode === 'window' ? item.version : (item.windowVersion || 1) }}
            <el-tag v-if="mode === 'clearance' && versionDrift(item)" size="small" type="danger" effect="plain" class="drift-tag">版本已变化</el-tag>
          </dd>
          <template v-if="mode === 'clearance'">
            <dt>关联窗口</dt>
            <dd :class="{ 'window-unsafe': windowUnsafe(item) }">{{ windowLabel(item) }}</dd>
            <dt>首次提交</dt><dd>{{ item.submittedBy || '待提交' }}</dd>
            <dt>独立复核</dt><dd>{{ item.confirmedBy || '待复核' }}</dd>
          </template>
          <template v-else>
            <dt>风险等级</dt><dd>{{ item.riskLevel }}</dd>
            <dt>评估证据</dt><dd>{{ item.evidence || '待补充' }}</dd>
          </template>
        </dl>
        <el-button v-if="mode === 'clearance' && canAct(item)" type="primary" @click="emit('confirm', { ...item, windowVersion: effectiveWindowVersion(item) })">{{ actionLabel(item) }}</el-button>
        <small v-else-if="mode === 'clearance' && blockedReason(item)" class="blocked-reason">{{ blockedReason(item) }}</small>
      </article>
    </div>
  </section>
</template>

<style scoped>
.drift-tag { margin-left: 6px; }
.window-unsafe { color: #c84855; font-weight: 700; }
.blocked-reason { color: #b5561f; display: block; line-height: 1.5; }
</style>
