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
    records.sort((left, right) => Number(right.status === 'pending' || right.rebindRequired) - Number(left.status === 'pending' || left.rebindRequired));
  }
  return records.slice(0, 6);
});

function canAct(item: DomainRecord): boolean {
  if (props.mode !== 'clearance' || item.status !== 'pending') return false;
  // 窗口换绑后，旧提交不能直接放行：必须先由原提交人（或管理员）基于新版本
  // 重新确认，再走独立复核。
  if (item.rebindRequired) {
    return canSubmit.value && (item.submittedBy === session.value?.username || session.value?.role === 'admin');
  }
  if (!item.submittedBy) return canSubmit.value;
  return canReview.value && item.submittedBy !== session.value?.username;
}

function actionLabel(item: DomainRecord): string {
  if (item.rebindRequired) return '重新确认（换绑新版本）';
  return item.submittedBy ? '复核并放行' : '提交安全确认';
}
</script>

<template>
  <section class="clearance-panel" aria-label="安全许可协同面板">
    <header>
      <div><span class="eyebrow">TWO-PERSON SAFETY</span><strong>{{ mode === 'window' ? '窗口许可依据' : '双人安全确认' }}</strong></div>
      <small v-if="mode === 'window'">窗口转差将同步过期已放行许可并换绑待复核许可</small>
      <small v-else>放行前校验窗口安全与版本；不安全或版本变化时必须重新确认</small>
    </header>
    <p v-if="mode === 'clearance'" class="clearance-hint">
      许可放行须同时满足：绑定风浪窗口处于「安全」状态、许可版本与窗口最新版本一致。窗口受限/过期会同步过期已放行许可；待复核许可换绑新版本后，旧提交不能直接放行，须由原提交人重新确认、再经他人独立复核。
    </p>
    <div class="clearance-grid">
      <article v-for="item in displayedRecords" :key="item.id" :class="{ 'clearance-card--rebind': mode === 'clearance' && item.rebindRequired }">
        <div class="clearance-title"><strong>{{ item.code }}</strong><StatusBadge :status="item.status"/></div>
        <p>{{ item.name }}</p>
        <el-tag v-if="mode === 'clearance' && item.rebindRequired" type="warning" size="small">窗口已换版，待重新确认</el-tag>
        <dl>
          <dt>窗口版本</dt><dd>v{{ mode === 'window' ? item.version : (item.windowVersion || 1) }}</dd>
          <template v-if="mode === 'clearance'">
            <dt>首次提交</dt><dd>{{ item.submittedBy || '待提交' }}</dd>
            <dt>独立复核</dt><dd>{{ item.confirmedBy || '待复核' }}</dd>
          </template>
          <template v-else>
            <dt>风险等级</dt><dd>{{ item.riskLevel }}</dd>
            <dt>评估证据</dt><dd>{{ item.evidence || '待补充' }}</dd>
          </template>
        </dl>
        <el-button v-if="mode === 'clearance' && canAct(item)" :type="item.rebindRequired ? 'warning' : 'primary'" @click="emit('confirm', item)">{{ actionLabel(item) }}</el-button>
        <small v-else-if="mode === 'clearance' && item.status === 'pending' && item.submittedBy && !item.rebindRequired">等待其他复核员确认</small>
        <small v-else-if="mode === 'clearance' && item.rebindRequired">等待原提交人 {{ item.submittedBy }} 重新确认</small>
      </article>
    </div>
  </section>
</template>
