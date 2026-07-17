(function(){
  const $=(s,r=document)=>r.querySelector(s); const $$=(s,r=document)=>Array.from(r.querySelectorAll(s));
  const sprite=`<svg aria-hidden="true" style="position:absolute;width:0;height:0;overflow:hidden"><defs>
    <symbol id="app-search" viewBox="0 0 24 24"><circle cx="11" cy="11" r="7" fill="none" stroke="currentColor"/><path d="m16.4 16.4 4.1 4.1" fill="none" stroke="currentColor"/></symbol>
    <symbol id="app-library" viewBox="0 0 24 24"><path d="M4 4v16M9 4v16M14 6l4-1 3 14-4 1z" fill="none" stroke="currentColor"/></symbol>
    <symbol id="app-heart" viewBox="0 0 24 24"><path d="M20.8 5.8a5.4 5.4 0 0 0-7.7 0L12 6.9l-1.1-1.1a5.4 5.4 0 1 0-7.7 7.7L12 22l8.8-8.5a5.4 5.4 0 0 0 0-7.7Z" fill="none" stroke="currentColor"/></symbol>
    <symbol id="app-list" viewBox="0 0 24 24"><path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" fill="none" stroke="currentColor"/></symbol>
    <symbol id="app-download" viewBox="0 0 24 24"><path d="M12 3v12m0 0 5-5m-5 5-5-5M4 21h16" fill="none" stroke="currentColor"/></symbol>
    <symbol id="app-settings" viewBox="0 0 24 24"><circle cx="12" cy="12" r="3" fill="none" stroke="currentColor"/><path d="M4.9 4.9 7.2 7M16.8 17l2.3 2.1M19.1 4.9 17 7.2M7.2 17l-2.3 2.1M12 2v3m0 14v3M2 12h3m14 0h3" fill="none" stroke="currentColor"/></symbol>
    <symbol id="app-play" viewBox="0 0 24 24"><path d="m8 5 11 7-11 7Z" fill="none" stroke="currentColor"/></symbol>
    <symbol id="app-pause" viewBox="0 0 24 24"><path d="M8 5v14M16 5v14" fill="none" stroke="currentColor"/></symbol>
    <symbol id="app-more" viewBox="0 0 24 24"><circle cx="5" cy="12" r="1" fill="currentColor"/><circle cx="12" cy="12" r="1" fill="currentColor"/><circle cx="19" cy="12" r="1" fill="currentColor"/></symbol>
    <symbol id="app-plus" viewBox="0 0 24 24"><path d="M12 5v14M5 12h14" fill="none" stroke="currentColor"/></symbol>
    <symbol id="app-arrow" viewBox="0 0 24 24"><path d="m9 18 6-6-6-6" fill="none" stroke="currentColor"/></symbol>
  </defs></svg>`;
  document.body.insertAdjacentHTML('afterbegin',sprite);
  $$('use[href*="assets/sprite.svg#"]').forEach(use=>{
    const id=(use.getAttribute('href')||'').split('#').pop();
    use.setAttribute('href',`#app-${id}`);
  });
  $$('.mobile-nav a[href="settings.html"] span').forEach(label=>label.textContent='Настройки');
  const toast=(title,detail='')=>{let el=$('#toast');if(!el)return;const strong=document.createElement('strong');strong.textContent=title;el.replaceChildren(strong);if(detail){const span=document.createElement('span');span.textContent=detail;el.append(span)}el.classList.add('show');clearTimeout(window.__toast);window.__toast=setTimeout(()=>el.classList.remove('show'),2600)}; window.appToast=toast;
  $$('.chip').forEach(b=>b.addEventListener('click',()=>b.classList.toggle('active')));
  $$('.switch').forEach(b=>b.addEventListener('click',()=>{b.classList.toggle('on');b.setAttribute('aria-checked',b.classList.contains('on'));toast(b.classList.contains('on')?'Настройка включена':'Настройка выключена')}));
  $$('.js-favorite').forEach(b=>b.addEventListener('click',()=>{b.classList.toggle('active');toast(b.classList.contains('active')?'Добавлено в избранное':'Удалено из избранного')}));
  $$('.js-download').forEach(b=>b.addEventListener('click',()=>toast('Задание создано','Скачивание — отдельное действие. Текущий stream не сохраняется.')));
  $$('.js-play').forEach(b=>b.addEventListener('click',()=>{const toggle=b.classList.contains('play')||b.classList.contains('primary');const next=toggle?!document.body.classList.contains('playing'):true;document.body.classList.toggle('playing',next);b.setAttribute('aria-label',next?'Пауза':'Воспроизвести');const use=b.querySelector('use');if(use){const href=use.getAttribute('href')||'';use.setAttribute('href',href.startsWith('#i-')?(next?'#i-pause':'#i-play'):(next?'#app-pause':'#app-play'))}toast(next?'Поток готов':'Воспроизведение приостановлено',next?'Playback contract обновлён.':'Позиция сохранена локально.')}));
  const settings=$$('.settings-tabs [data-settings-tab]');
  const panels=$$('[data-settings-panel]');
  const showSettings=id=>{
    settings.forEach(b=>{const active=b.dataset.settingsTab===id;b.classList.toggle('active',active);b.setAttribute('aria-selected',String(active))});
    panels.forEach(panel=>panel.hidden=panel.dataset.settingsPanel!==id);
    if(history.replaceState)history.replaceState(null,'',`#${id}`);
  };
  settings.forEach(b=>b.addEventListener('click',()=>showSettings(b.dataset.settingsTab)));
  if(settings.length){const requested=location.hash.slice(1);showSettings(settings.some(b=>b.dataset.settingsTab===requested)?requested:settings[0].dataset.settingsTab)}
  const actionLabels={play:'Воспроизвести',heart:'Добавить в избранное',more:'Дополнительные действия',arrow:'Открыть подробности',download:'Сохранить offline'}; $$('button').forEach(b=>{if(b.hasAttribute('aria-label')||b.textContent.trim())return;const href=b.querySelector('use')?.getAttribute('href')||'';const key=href.split('#').pop().replace('app-','');if(key&&actionLabels[key])b.setAttribute('aria-label',actionLabels[key])});
  const search=$('#global-search'); if(search){
    const run=()=>{const q=search.value.trim();if(!q){toast('Введите запрос','Raw query не отправляется в analytics.');return}toast('Поиск запущен',`Провайдеры отвечают на «${q}»`);const label=$('#result-label'),table=$('#search-results');if(label)label.textContent='Источники отвечают…';if(table){table.classList.add('is-loading');table.setAttribute('aria-busy','true')}setTimeout(()=>{if(label)label.textContent=`Результаты для «${q}»`;if(table){table.classList.remove('is-loading');table.setAttribute('aria-busy','false')}},650)};
    search.addEventListener('keydown',e=>{if(e.key==='Enter')run()});
    window.addEventListener('keydown',e=>{if((e.metaKey||e.ctrlKey)&&e.key.toLowerCase()==='k'||(e.key==='/'&&!/input|textarea/i.test(e.target.tagName))){e.preventDefault();search.focus();}});
  }
})();
