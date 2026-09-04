<script lang="ts">
	import { fade } from 'svelte/transition';
	import QueueList from './QueueList.svelte';
	import { t } from '$lib/i18n.svelte';

	let { onClose }: { onClose: () => void } = $props();
</script>


<button
	class="absolute inset-0 z-20 cursor-default bg-black/40 lg:hidden"
	onclick={onClose}
	aria-label={t('Close queue')}
	transition:fade={{ duration: 90 }}
></button>
<aside
	transition:fade={{ duration: 210 }}
	class="ryo-float-panel absolute inset-y-0 right-0 z-30 flex h-full w-80 max-w-[80vw] flex-col border-l bg-card"
>
	<h2 class="ryo-panel-title border-b px-4 py-3">{t('// QUEUE')}</h2>
	<QueueList />
</aside>
