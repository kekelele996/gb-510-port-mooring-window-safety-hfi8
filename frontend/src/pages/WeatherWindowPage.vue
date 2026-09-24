<script setup lang="ts">
import { ref, watch } from 'vue';
import EntityPage from '../components/EntityPage.vue';
import RiskBadge from '../components/common/RiskBadge.vue';
import ClearancePanel from '../components/common/ClearancePanel.vue';
import ImpactAssessmentDialog from '../components/common/ImpactAssessmentDialog.vue';
import { ENTITY_CONFIGS } from '../types/status';
import { useWeatherWindowStore } from '../stores/weather-window';
import { useSafetyClearanceStore } from '../stores/safety-clearance';
import { getWindowImpact } from '../api/weather-window';
import type { DomainRecord, WindowImpactAssessment } from '../types/domain';

const store = useWeatherWindowStore();
const clearanceStore = useSafetyClearanceStore();
const impactVisible = ref(false);
const impactLoading = ref(false);
const assessment = ref<WindowImpactAssessment | null>(null);

// A window downgrade synchronously expires/rebinds clearances; mirror the
// server-side cascade in the clearance store whenever window data changes.
watch(() => store.items.map((item) => `${item.id}:${item.version}:${item.status}`).join('|'), () => {
  void clearanceStore.load('clearance');
});

async function openImpact(item: DomainRecord) {
  impactVisible.value = true;
  impactLoading.value = true;
  assessment.value = null;
  try {
    const result = await getWindowImpact(item.id);
    assessment.value = result.data;
  } finally {
    impactLoading.value = false;
  }
}
</script>

<template>
  <EntityPage :config="ENTITY_CONFIGS[2]" :store="store">
    <template #insight>
      <RiskBadge :records="store.items"/>
      <ClearancePanel :records="store.items" mode="window"/>
    </template>
    <template #row-actions="{ row }">
      <el-button link type="warning" @click="openImpact(row)">影响评估</el-button>
    </template>
  </EntityPage>
  <ImpactAssessmentDialog v-model="impactVisible" :assessment="assessment" :loading="impactLoading"/>
</template>
