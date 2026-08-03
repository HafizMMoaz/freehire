<script lang="ts">
  import { resolve } from '$app/paths';
  import { Dialog } from '$lib/ui';

  // Points a candidate's own coding agent at this CV. The commands mirror the
  // "Tailor a CV to a vacancy" block on /cli exactly, with this CV's own id filled
  // in — copy-paste-ready rather than another <id> the reader has to substitute.
  let { cvId, onClose }: { cvId: string; onClose: () => void } = $props();

  let open = $state(true);
  $effect(() => {
    if (!open) onClose();
  });
</script>

<Dialog bind:open title="Edit this CV from the CLI" class="max-w-lg">
  <p class="text-sm leading-relaxed text-muted-foreground">
    Give these commands to your own coding agent — it reads and writes this exact CV the same way
    the in-app assistant does.
  </p>
  <pre
    class="mt-3 overflow-x-auto rounded-md border border-border bg-background/60 p-3 font-mono text-sm leading-relaxed"><span class="text-muted-foreground">freehire</span> cv context {cvId}        <span class="text-muted-foreground"># the fit analysis to reframe toward</span>
<span class="text-muted-foreground">freehire</span> cv get {cvId}            <span class="text-muted-foreground"># the CV document as JSON</span>
<span class="text-muted-foreground">freehire</span> cv edit {cvId} --set …   <span class="text-muted-foreground"># one edit by path (--ops for a batch)</span>
<span class="text-muted-foreground">freehire</span> cv render {cvId} --out cv.pdf</pre>
  <p class="mt-3 text-sm leading-relaxed text-muted-foreground">
    You'll need your own
    <a
      href={resolve('/my/api-keys')}
      class="font-medium text-foreground underline-offset-4 hover:underline">API key</a
    > first, then install and sign in the CLI — full setup and the rest of the commands (search,
    tracking, MCP) are on the
    <a href={resolve('/cli')} class="font-medium text-foreground underline-offset-4 hover:underline"
      >CLI reference</a
    >.
  </p>
</Dialog>
