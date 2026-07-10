<template>
  <span
    :class="[
      'inline-flex max-w-full items-center gap-1 rounded-md border px-2 py-0.5 text-[11px] font-medium uppercase',
      platformBadgeClass(platform),
    ]"
  >
    <PlatformIcon :platform="platform as GroupPlatform" size="xs" />
    <span class="truncate">{{ platform }}</span>
  </span>
</template>

<script setup lang="ts">
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import { platformBadgeClass } from '@/utils/platformColors'

defineProps<{ platform: string }>()
</script>
