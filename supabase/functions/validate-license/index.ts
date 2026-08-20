import { serve } from "https://deno.land/std@0.168.0/http/server.ts"
import { createClient } from "https://esm.sh/@supabase/supabase-js@2"
const corsHeaders = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Headers': 'authorization, x-client-info, apikey, content-type',
};

serve(async (req) => {
  if (req.method === 'OPTIONS') {
    return new Response('ok', { headers: corsHeaders })
  }

  try {
    const { license, hwid } = await req.json()
    if (!license || !hwid) {
      return new Response(JSON.stringify({ valid: false, reason: "missing_fields" }), {
        status: 400,
        headers: { ...corsHeaders, "Content-Type": "application/json" }
      })
    }

    // Create a Supabase client with the Auth context of the logged in user.
    // We use the service_role key to bypass RLS and act authoritatively.
    const supabaseUrl = Deno.env.get('SUPABASE_URL') ?? '';
    const supabaseServiceKey = Deno.env.get('SUPABASE_SERVICE_ROLE_KEY') ?? '';
    const supabase = createClient(supabaseUrl, supabaseServiceKey);

    // 1. Check if the license exists
    const { data: lic, error: licError } = await supabase
      .from('licenses')
      .select('*')
      .eq('license_key', license.trim())
      .single()

    if (licError || !lic) {
      return new Response(JSON.stringify({ valid: false, reason: "invalid_license" }), {
        status: 200,
        headers: { ...corsHeaders, "Content-Type": "application/json" }
      })
    }

    // 2. Check status and expiration
    if (lic.status !== 'active') {
      return new Response(JSON.stringify({ valid: false, reason: "license_" + lic.status }), {
        status: 200,
        headers: { ...corsHeaders, "Content-Type": "application/json" }
      })
    }
    
    if (lic.expires_at && new Date(lic.expires_at) < new Date()) {
      return new Response(JSON.stringify({ valid: false, reason: "license_expired" }), {
        status: 200,
        headers: { ...corsHeaders, "Content-Type": "application/json" }
      })
    }

    // 3. HWID Binding and Validation
    if (lic.hwid) {
      // It's already bound
      if (lic.hwid !== hwid) {
        return new Response(JSON.stringify({ valid: false, reason: "hwid_mismatch" }), {
          status: 200,
          headers: { ...corsHeaders, "Content-Type": "application/json" }
        })
      }
      // Update last seen
      await supabase
        .from('licenses')
        .update({ last_seen_at: new Date().toISOString() })
        .eq('id', lic.id)
        
    } else {
      // First time activation - bind it
      const { error: updateError } = await supabase
        .from('licenses')
        .update({
          hwid: hwid,
          activated_at: new Date().toISOString(),
          last_seen_at: new Date().toISOString()
        })
        .eq('id', lic.id)
        .is('hwid', null) // Concurrency check: ensure it wasn't claimed by someone else in the last few ms
        
      if (updateError) {
        return new Response(JSON.stringify({ valid: false, reason: "activation_failed" }), {
          status: 500,
          headers: { ...corsHeaders, "Content-Type": "application/json" }
        })
      }
    }

    return new Response(JSON.stringify({
      valid: true,
      license_id: lic.id,
      expires_at: lic.expires_at
    }), {
      status: 200,
      headers: { ...corsHeaders, "Content-Type": "application/json" }
    })

  } catch (err) {
    return new Response(JSON.stringify({ valid: false, reason: "server_error" }), {
      status: 500,
      headers: { ...corsHeaders, "Content-Type": "application/json" }
    })
  }
})
