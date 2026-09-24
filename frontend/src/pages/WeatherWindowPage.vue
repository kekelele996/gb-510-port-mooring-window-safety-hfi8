<script setup lang="ts">
import { ref } from 'vue';
import EntityPage from '../components/EntityPage.vue';
import RiskBadge from '../components/common/RiskBadge.vue';
import ClearancePanel from '../components/common/ClearancePanel.vue';
import ImpactPanel from '../components/common/ImpactPanel.vue';
import { ENTITY_CONFIGS } from '../types/status';
import { useWeatherWindowStore } from '../stores/weather-window';
import { getWindowImpact } from '../api/weather-window';
import type { DomainRecord, WindowImpact } from '../types/domain';

const store = useWeatherWindowStore();
const impactVisible = ref(false);
const impactLoading = ref(false);
const impact = ref<WindowImpact | null>(null);

async function openImpact(row: DomainRecord) {
  impactVisible.value = true;
  impactLoading.value = true;
  impact.value = null;
  try {
    const result = await getWindowImpact(row.id);
    impact.value = result.data;
  } catch (error) {
    store.error = error instanceof Error ? error.message : String(error);
  } finally {
    impactLoading.value = false;
  }
}
</script>

<template>
  <EntityPage :config="ENTITY_CONFIGS[2]" :store="store" :action-width="250">
    <template #insight>
      <RiskBadge :records="store.items"/>
      <ClearancePanel :records="store.items" mode="window"/>
    </template>
    <template #rowActions="{ row }">
      <el-button link type="primary" @click="openImpact(row)">影响评估</el-button>
    </template>
  </EntityPage>

  <el-drawer v-model="impactVisible" title="窗口影响评估" size="62%" destroy-on-close>
    <ImpactPanel :impact="impact" :loading="impactLoading"/>
    <template v-if="impact" #footer>
      <el-button @click="impactVisible = false">关闭</el-button>
      <el-button type="primary" @click="openImpact(impact.window)">刷新评估</el-button>
    </template>
  </el-drawer>
</template>
