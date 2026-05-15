<script lang="ts">
  import { navigate } from 'svelte-routing'
  import { auth } from './auth.svelte'

  let { adminOnly = false, children }: { adminOnly?: boolean; children: any } = $props()

  $effect(() => {
    if (!auth.loading) {
      if (!auth.isLoggedIn && !auth.isLocalMode) navigate('/login')
      else if (adminOnly && !auth.isAdmin) navigate('/')
    }
  })
</script>

{#if !auth.loading && (auth.isLoggedIn || auth.isLocalMode) && (!adminOnly || auth.isAdmin)}
  {@render children()}
{/if}
