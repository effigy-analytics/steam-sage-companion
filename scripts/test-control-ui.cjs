// Optional browser regression runner; no production Node/Playwright dependency.
// Run under scripts/with-isolated-network.sh with a synthetic HOME/XDG/PATH.
const fs = require('node:fs');
const path = require('node:path');
const {chromium} = require(process.env.COMPANION_PLAYWRIGHT_MODULE || 'playwright');
const assets = path.resolve(__dirname, '../internal/bridge/web');
const out = process.env.COMPANION_BROWSER_EVIDENCE;
if (process.env.STEAM_SAGE_ISOLATED_NETWORK !== '1') throw new Error('Use scripts/with-isolated-network.sh');
(async () => {
  const browser = await chromium.launch({executablePath:process.env.COMPANION_CHROMIUM,headless:true,args:['--no-sandbox']});
  const failures = [];
  async function run(name, fn, focused = true, colorScheme = 'light') {
    const context = await browser.newContext({viewport:{width:390,height:844},colorScheme});
    const page = await context.newPage();
    let state = {accounts:[{id:'synthetic-account',steam_id:'76561198000000001',location:'Steam',selected:true}],pending:[{request_id:'focused',origin:'https://steam-sage.effigy-analytics.com',expires_at:'2099-01-01T00:00:00Z'},{request_id:'other',origin:'https://another.example',expires_at:'2099-01-01T00:00:00Z'}],paired_origins:[],startup:true,selected_steam_id:'76561198000000001',source:{protocol:1,state:'ready',attempted_at:'2026-10-24T00:00:00Z',error:null,snapshot:{protocol:1,instance_id:'synthetic',source_id:'synthetic',steam_id:'76561198000000001',revision:'1',read_at:'2026-10-24T00:00:00Z',collections:[]}},focused_request:{request_id:'focused',origin:'https://steam-sage.effigy-analytics.com',expires_at:'2099-01-01T00:00:00Z',state:'pending'}};
    const actions = [];
    let releaseAction;
    const delayedAction = () => new Promise(resolve => {releaseAction = resolve;});
    const delay = {enabled:false, release:() => releaseAction()};
    await page.addInitScript(() => {window.setInterval = fn => {window.poll = fn;return 1;};});
    await page.route('**/*', async route => {
      const u = new URL(route.request().url());
      if (u.origin !== 'http://127.0.0.1:43871') throw new Error('Unexpected navigation');
      if (u.pathname === '/control/session') return route.fulfill({json:{control_token:'synthetic-private-session'}});
      if (u.pathname === '/control/state') {
        if (route.request().headers()['x-companion-control'] !== 'synthetic-private-session') throw new Error('Missing private control session');
        if (u.searchParams.get('request_id') !== (focused ? 'focused' : null)) throw new Error('Focused request not bound to private read');
        return route.fulfill({json:state});
      }
      if (u.pathname.startsWith('/control/')) {if(delay.enabled) await delayedAction(); actions.push({endpoint:u.pathname,...route.request().postDataJSON()}); return route.fulfill({json:{saved:true}});}
      const file = u.pathname === '/' ? 'index.html' : u.pathname.slice(1);
      return route.fulfill({body:fs.readFileSync(assets+'/'+file),contentType:file.endsWith('.js')?'text/javascript':file.endsWith('.css')?'text/css':'text/html'});
    });
    try {await page.goto('http://127.0.0.1:43871/#launch=synthetic-launch'+(focused?'&request_id=focused':''));await page.waitForFunction(()=>window.poll);await fn(page,state,actions,delay);console.log('PASS '+name);} catch(e) {failures.push(name+': '+e.message);console.log('FAIL '+name+': '+e.message);} finally {await context.close();}
  }
  await run('exact focused approval and lifecycle', async(page,state,actions)=>{
    const allow = page.getByRole('button',{name:'Allow collections',exact:true});
    if(await allow.count()!==1) throw new Error('Missing exact focused approval');
    if(await page.locator('#approval').textContent().then(s=>s.includes('another.example'))) throw new Error('Unrelated pending request in focused approval');
    if(new URL(page.url()).hash) throw new Error('Private fragment retained');
    if(!await page.locator('#startup').isChecked()) throw new Error('Existing startup preference lost');
    if(out) await page.screenshot({path:out+'/approval-390.png',fullPage:true});
    await allow.focus();await page.evaluate(()=>window.poll());await page.evaluate(()=>window.poll());
    if(!await allow.evaluate(el=>el===document.activeElement)) throw new Error('Approval polling lost keyboard focus');
    if(await page.getByRole('button',{name:'Allow collections',exact:true}).count()!==1) throw new Error('Focused request changed');
    await allow.click();await page.waitForFunction(()=>!document.querySelector('#allow').disabled);
    if(actions.at(-1)?.request_id!=='focused' || actions.at(-1)?.endpoint!=='/control/approve') throw new Error('Approved another request');
    for(const terminal of ['approved','claimed','cancelled','declined','expired']) {state.focused_request.state=terminal;state.pending=state.pending.filter(p=>p.request_id!=='focused');await page.evaluate(()=>window.poll());if(await allow.isVisible()) throw new Error('Terminal request still actionable: '+terminal);if(!await page.locator('#approval').textContent().then(t=>t.toLowerCase().includes(terminal==='claimed'?'connected':terminal==='approved'?'approved':terminal))) throw new Error('Missing terminal explanation: '+terminal);}
    delete state.focused_request;await page.evaluate(()=>window.poll());if(await allow.isVisible()) throw new Error('Missing focused request switched to other request');
  });
  await run('approval stays disabled through polling while its response is pending',async(page,state,actions,delay)=>{
    delay.enabled=true;
    const allow=page.locator('#allow');
    const request=page.waitForRequest(r=>r.url().endsWith('/control/approve'));
    await allow.click();await request;
    await page.evaluate(()=>window.poll());
    if(!await allow.isDisabled()) throw new Error('Polling reenabled an approval with a pending response');
    delay.release();
    await page.waitForFunction(()=>!document.querySelector('#allow').disabled);
    if(actions.length!==1||actions[0].request_id!=='focused') throw new Error('Busy action lost exact-request binding');
  });
  await run('polling retains existing approval keyboard focus',async(page)=>{const allow=page.getByRole('button',{name:/Allow/}).first();await allow.focus();await page.evaluate(()=>window.poll());await page.evaluate(()=>window.poll());if(!await allow.evaluate(el=>el===document.activeElement)) throw new Error('Approval polling lost keyboard focus');});
  await run('missing selection versus unreadable selected source',async(page,state)=>{state.source={protocol:1,state:'unavailable',attempted_at:'2026-10-24T00:00:00Z',error:'unreadable',snapshot:null};await page.evaluate(()=>window.poll());const selected=await page.locator('#freshness').textContent();if(/choose an account/i.test(selected)||!/(could not|couldn.t|unable).*read/i.test(selected)) throw new Error('Unreadable selected data incorrectly described as missing selection');state.selected_steam_id='';delete state.source;await page.evaluate(()=>window.poll());if(!/choose an account/i.test(await page.locator('#freshness').textContent())) throw new Error('Missing account selection not explained');});
  await run('decline stays bound to the original request',async(page,state,actions)=>{
    await page.getByRole('button',{name:'Not now',exact:true}).click();
    await page.waitForFunction(()=>!document.querySelector('#decline').disabled);
    if(actions.at(-1)?.request_id!=='focused'||actions.at(-1)?.endpoint!=='/control/decline') throw new Error('Declined another request');
    state.focused_request.state='declined';await page.evaluate(()=>window.poll());
    if(await page.getByRole('button',{name:'Allow collections',exact:true}).isVisible()) throw new Error('Declined request remains actionable');
  });
  await run('settings remains usable without a focus fragment',async(page,state,actions)=>{
    if(!await page.locator('#settings').evaluate(el=>el.open)) throw new Error('Settings not exposed');
    if(await page.locator('#approval').isVisible()) throw new Error('Settings presented as one-time permission');
    await page.locator('#root').evaluate(el=>el.closest('details').open=true);await page.locator('#root').fill('/synthetic/Steam');await page.locator('#root-save').click();
    await page.waitForFunction(()=>!document.querySelector('#root-save').disabled);
    if(actions.at(-1)?.endpoint!=='/control/root'||actions.at(-1)?.path!=='/synthetic/Steam') throw new Error('Folder recovery unavailable');
    await page.locator('#accounts button').focus();await page.evaluate(()=>window.poll());await page.evaluate(()=>window.poll());
    if(!await page.locator('#accounts button').evaluate(el=>el===document.activeElement)) throw new Error('Account focus lost on polling');
    if(await page.locator('#root').inputValue()!=='/synthetic/Steam') throw new Error('Polling erased manual folder input');
  }, false);
  await run('unselected account disables approval and preserves startup choice',async(page,state)=>{
    state.selected_steam_id='';state.accounts[0].selected=false;state.startup=false;delete state.source;await page.evaluate(()=>window.poll());
    if(!await page.locator('#allow').isDisabled()) throw new Error('Approval enabled without account');
    if(await page.locator('#startup').isChecked()) throw new Error('Startup enabled automatically');
    const order=await page.evaluate(()=>!!(document.querySelector('#accounts').compareDocumentPosition(document.querySelector('#allow'))&Node.DOCUMENT_POSITION_FOLLOWING));
    if(!order) throw new Error('Account selection follows approval');
  });
  await run('sanitized first-read recovery and stale read',async(page,state)=>{
    state.source={protocol:1,state:'unavailable',attempted_at:'2026-10-24T00:00:00Z',error:'/private/Steam/raw-data-secret',snapshot:null};await page.evaluate(()=>window.poll());
    if((await page.locator('body').textContent()).includes('raw-data-secret')) throw new Error('Raw source failure leaked');
    await page.locator('#settings').evaluate(el=>el.open=true);
    if(!await page.locator('#choose').isVisible()) throw new Error('Recovery folder controls unavailable');
    state.source={protocol:1,state:'stale',attempted_at:'2026-10-24T00:00:00Z',error:'unreadable',snapshot:{protocol:1,instance_id:'synthetic',source_id:'synthetic',steam_id:'76561198000000001',revision:'1',read_at:'2026-10-24T00:00:00Z',collections:[]}};await page.evaluate(()=>window.poll());
    if(!/keeping the last successful read/i.test(await page.locator('#freshness').textContent())) throw new Error('Stale read not explained');
    await page.setViewportSize({width:1280,height:900});
    if(out) await page.screenshot({path:out+'/approval-recovery-dark-desktop.png',fullPage:true});
  },true,'dark');
  await browser.close();if(failures.length) process.exitCode=1;
})();
