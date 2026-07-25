<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';

  let {
    label,
    error,
    hint,
    required = false,
    class: className,
    children,
  }: {
    label?: string;
    error?: string;
    hint?: string;
    required?: boolean;
    class?: string;
    /**
     * Receives everything the control has to carry itself — the field only owns
     * the label and the message, so `required` and the error state are the
     * control's to announce:
     * `{@render children({ id, describedBy, required, invalid })}`.
     */
    children: Snippet<
      [{ id: string; describedBy: string | undefined; required: boolean; invalid: boolean }]
    >;
  } = $props();

  // $props.id() is stable across SSR and hydration — crypto.randomUUID() is not.
  const uid = $props.id();
  const id = `${uid}-control`;
  const messageId = `${uid}-message`;
  let describedBy = $derived(error || hint ? messageId : undefined);
  let invalid = $derived(Boolean(error));
</script>

<div class={cn('flex flex-col gap-1.5', className)}>
  {#if label}
    <label for={id} class="text-sm font-medium">
      {label}
      {#if required}
        <!-- Decorative: the control carries `required`, so reading "asterisk" here
             would only duplicate it. -->
        <span class="text-destructive" aria-hidden="true">*</span>
      {/if}
    </label>
  {/if}
  {@render children({ id, describedBy, required, invalid })}
  {#if error}
    <p id={messageId} class="text-sm text-destructive" role="alert">{error}</p>
  {:else if hint}
    <p id={messageId} class="text-sm text-muted-foreground">{hint}</p>
  {/if}
</div>
