<script setup lang="ts">
import { computed, ref } from "vue";
import {
  type DayRow,
  buildMonthCells,
  cellClass,
  monthTitle,
  shanghaiTodayISO,
  shiftMonth,
} from "../calendarMonth";

const props = defineProps<{
  days: DayRow[];
  displayDate?: string;
}>();

const today = shanghaiTodayISO();
const initial = today.split("-").map(Number);
const viewYear = ref(initial[0]);
const viewMonth = ref(initial[1]);

const title = computed(() => monthTitle(viewYear.value, viewMonth.value));
const cells = computed(() =>
  buildMonthCells({
    year: viewYear.value,
    month: viewMonth.value,
    days: props.days,
    today,
    displayDate: props.displayDate,
  }),
);

function prev() {
  const n = shiftMonth(viewYear.value, viewMonth.value, -1);
  viewYear.value = n.year;
  viewMonth.value = n.month;
}

function next() {
  const n = shiftMonth(viewYear.value, viewMonth.value, 1);
  viewYear.value = n.year;
  viewMonth.value = n.month;
}
</script>

<template>
  <div class="cal">
    <div class="cal-nav">
      <button type="button" class="btn ghost" @click="prev">上一月</button>
      <div class="cal-title">{{ title }}</div>
      <button type="button" class="btn ghost" @click="next">下一月</button>
    </div>
    <div class="cal-weekdays">
      <div v-for="w in ['一', '二', '三', '四', '五', '六', '日']" :key="w">{{ w }}</div>
    </div>
    <div class="cal-grid">
      <div v-for="c in cells" :key="c.key" :class="cellClass(c)">
        <template v-if="c.kind !== 'pad'">
          <div class="cal-day">{{ c.day }}</div>
          <div v-if="c.kind !== 'blank'" class="cal-amt mono">{{ c.pnl }}</div>
          <div v-if="c.unitNav" class="mono" :class="c.kind === 'hang_zero' ? 'cal-hang-label' : 'cal-nav-label'">{{ c.unitNav }}</div>
        </template>
      </div>
    </div>
  </div>
</template>
