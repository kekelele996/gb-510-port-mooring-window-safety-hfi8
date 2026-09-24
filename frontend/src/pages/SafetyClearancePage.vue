<script setup lang="ts">
import { ElMessageBox } from 'element-plus';
import EntityPage from '../components/EntityPage.vue';
import ClearancePanel from '../components/common/ClearancePanel.vue';
import type { DomainRecord } from '../types/domain';
import { ENTITY_CONFIGS } from '../types/status';
import { useSafetyClearanceStore } from '../stores/safety-clearance';

const store = useSafetyClearanceStore();

async function confirmClearance(item: DomainRecord) {
  const rebind = Boolean(item.rebindRequired);
  try {
    await ElMessageBox.confirm(
      rebind
        ? `许可 ${item.code} 绑定的风浪窗口已换版本（当前 v${item.windowVersion || 1}），旧提交不能直接放行。确认由原提交人基于当前窗口安全状态与最新版本重新确认？`
        : `放行前请确认：${item.code} 绑定的风浪窗口当前为「安全」状态，且许可版本与窗口最新版本一致。若窗口不安全或版本已变化，系统将拒绝放行并要求重新确认。是否继续？`,
      rebind ? '窗口换版，重新确认' : '放行前窗口复核',
      { confirmButtonText: rebind ? '基于新版本重新确认' : '确认并继续', cancelButtonText: '取消', type: rebind ? 'warning' : 'info' },
    );
  } catch {
    return;
  }
  await store.confirmClearance('clearance', item);
}
</script>

<template>
  <EntityPage :config="ENTITY_CONFIGS[3]" :store="store" hide-transitions>
    <template #insight><ClearancePanel :records="store.items" mode="clearance" @confirm="confirmClearance"/></template>
  </EntityPage>
</template>
