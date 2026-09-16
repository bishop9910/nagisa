<script setup lang="ts">
/** 预览：图片、视频、音频、PDF 与文本；其余类型提示下载。 */
import { computed, ref, watch } from 'vue'

import AppButton from '@/components/ui/AppButton.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import { errorText, filesApi } from '@/api'
import type { Node } from '@/api/types'
import { formatBytes } from '@/utils/format'
import { previewModeOf } from '@/utils/media'

const props = defineProps<{
  modelValue: boolean
  node?: Node | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'download', node: Node): void
}>()

const loading = ref(false)
const error = ref('')
const url = ref('')
const textContent = ref('')

const mode = computed(() => previewModeOf(props.node))
/** 文本预览只取前 512 KiB，避免把大日志整块读进内存。 */
const TEXT_LIMIT = 512 * 1024

async function load(): Promise<void> {
  if (!props.node?.id) return
  loading.value = true
  error.value = ''
  url.value = ''
  textContent.value = ''
  try {
    const signed = await filesApi.getPreviewUrl(props.node.id, { expiresInSeconds: 900 })
    url.value = signed.url ?? ''
    if (!url.value) throw new Error('服务端未返回预览地址')
    if (mode.value === 'text') {
      const response = await fetch(url.value)
      if (!response.ok) throw new Error(`读取内容失败（HTTP ${response.status}）`)
      const blob = await response.blob()
      textContent.value = await blob.slice(0, TEXT_LIMIT).text()
    }
  } catch (err) {
    error.value = errorText(err)
  } finally {
    loading.value = false
  }
}

function openInNewTab(): void {
  if (url.value) window.open(url.value, '_blank', 'noopener')
}

watch(
  () => [props.modelValue, props.node?.id],
  ([open]) => {
    if (open) void load()
    else {
      url.value = ''
      textContent.value = ''
      error.value = ''
    }
  },
)
</script>

<template>
  <AppDialog
    :model-value="modelValue"
    :title="node?.name || '预览'"
    size="xl"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="preview">
      <p class="preview__meta">
        <span>{{ node?.mimeType || '未知类型' }}</span>
        <span>·</span>
        <span>{{ formatBytes(node?.size) }}</span>
        <span v-if="node?.ownerName">· {{ node.ownerName }}</span>
      </p>

      <div v-if="loading" class="preview__stage">
        <AppSpinner :size="26" label="正在获取预览…" />
      </div>
      <div v-else-if="error" class="preview__stage preview__stage--error">
        <AppIcon name="alert-circle" :size="26" />
        <p>{{ error }}</p>
      </div>
      <div v-else class="preview__stage">
        <img v-if="mode === 'image'" :src="url" :alt="node?.name" class="preview__image" />
        <video v-else-if="mode === 'video'" :src="url" class="preview__video" controls playsinline />
        <audio v-else-if="mode === 'audio'" :src="url" class="preview__audio" controls />
        <iframe v-else-if="mode === 'pdf'" :src="url" class="preview__frame" title="PDF 预览" />
        <pre v-else-if="mode === 'text'" class="preview__text">{{ textContent }}</pre>
        <div v-else class="preview__none">
          <AppIcon name="file" :size="26" />
          <p>该类型不支持在线预览</p>
          <AppButton variant="primary" icon="download" @click="node && emit('download', node)">下载文件</AppButton>
        </div>
      </div>
    </div>

    <template #footer>
      <AppButton variant="ghost" @click="emit('update:modelValue', false)">关闭</AppButton>
      <AppButton v-if="node" icon="download" @click="emit('download', node)">下载</AppButton>
      <AppButton v-if="url" variant="primary" icon="external" @click="openInNewTab">新窗口打开</AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.preview {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.preview__meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--text-2xs);
  color: var(--text-muted);
}

.preview__stage {
  min-height: 320px;
  max-height: 62vh;
  background: var(--bg-sunken);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: auto;
  padding: var(--space-3);
}

.preview__stage--error {
  flex-direction: column;
  gap: var(--space-2);
  color: var(--danger-600);
  font-size: var(--text-sm);
}

.preview__image {
  max-width: 100%;
  max-height: 60vh;
  object-fit: contain;
  border-radius: var(--radius-sm);
}

.preview__video {
  width: 100%;
  max-height: 60vh;
  border-radius: var(--radius-sm);
  background: #000;
}

.preview__audio {
  width: min(520px, 100%);
}

.preview__frame {
  width: 100%;
  height: 60vh;
  border: 0;
  border-radius: var(--radius-sm);
  background: #fff;
}

.preview__text {
  width: 100%;
  margin: 0;
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--text);
}

.preview__none {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-3);
  color: var(--text-muted);
  font-size: var(--text-sm);
}
</style>
