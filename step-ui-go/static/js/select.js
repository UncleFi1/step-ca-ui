/*
 * select.js — автообёртка <select> в кастомный UI
 * Подключается из base.html и admin_base.html
 */

// Чистим устаревший ключ темы (теперь хранится на сервере)
try{localStorage.removeItem('step-ui-theme');}catch(e){}
var _sidebarOpen = true;
(function(){
  var saved = localStorage.getItem('step-ui-sidebar');
  if(saved === '0'){
    _sidebarOpen = false;
    document.getElementById('sidebar').classList.add('collapsed');
    document.body.classList.add('sb-collapsed');
    var bf = document.getElementById('burgerFloat');
    if(bf) bf.classList.add('visible');
  }
})();
function toggleSidebar(){
  _sidebarOpen = !_sidebarOpen;
  document.getElementById('sidebar').classList.toggle('collapsed', !_sidebarOpen);
  document.body.classList.toggle('sb-collapsed', !_sidebarOpen);
  var bf = document.getElementById('burgerFloat');
  if(bf) bf.classList.toggle('visible', !_sidebarOpen);
  localStorage.setItem('step-ui-sidebar', _sidebarOpen ? '1' : '0');
}

function toggleGroup(id){
  var grp = document.getElementById(id);
  if(!grp) return;
  var toggle = grp.querySelector('.nav-group-toggle');
  var items  = grp.querySelector('.nav-group-items');
  toggle.classList.toggle('open');
  items.classList.toggle('open');
}

function initCustomSelect(wrap){
  var sel = wrap.querySelector('select');
  if(!sel) return;
  var multi = sel.multiple;
  var box   = wrap.querySelector('.sel-box');
  var drop  = wrap.querySelector('.sel-dropdown');
  if(!box || !drop) return;
  function renderLabel(){
    var chosen = Array.from(sel.selectedOptions).map(function(o){return o.text;});
    var lbl = box.querySelector('.sel-label');
    if(lbl) lbl.textContent = chosen.length ? chosen.join(', ') : (sel.options[0]||{text:''}).text;
  }
  function buildOpts(){
    drop.innerHTML = '';
    Array.from(sel.options).forEach(function(o){
      var div = document.createElement('div');
      div.className = 'sel-opt' + (o.selected ? ' selected' : '');
      div.dataset.val = o.value;
      if(multi){
        div.innerHTML = '<span class="chk">'+(o.selected?'&#10003;':'')+'</span>'+o.text;
      } else {
        div.innerHTML = '<span class="sel-check">'+(o.selected?'&#10003;':'')+'</span>'+o.text;
      }
      div.addEventListener('mousedown', function(e){
        e.preventDefault();
        if(multi){
          o.selected = !o.selected;
          div.classList.toggle('selected', o.selected);
          var chk = div.querySelector('.chk');
          if(chk) chk.innerHTML = o.selected ? '&#10003;' : '';
        } else {
          Array.from(drop.querySelectorAll('.sel-opt')).forEach(function(d){
            d.classList.remove('selected');
            var sc = d.querySelector('.sel-check'); if(sc) sc.innerHTML='';
          });
          div.classList.add('selected');
          var sc = div.querySelector('.sel-check'); if(sc) sc.innerHTML='&#10003;';
          sel.value = o.value;
          closeDropdown();
        }
        renderLabel();
        sel.dispatchEvent(new Event('change',{bubbles:true}));
      });
      drop.appendChild(div);
    });
  }
  function openDropdown(){ buildOpts(); box.classList.add('open'); drop.classList.add('open'); }
  function closeDropdown(){ box.classList.remove('open'); drop.classList.remove('open'); }
  box.addEventListener('click', function(){ drop.classList.contains('open') ? closeDropdown() : openDropdown(); });
  document.addEventListener('click', function(e){ if(!wrap.contains(e.target)) closeDropdown(); });
  renderLabel();
}
// Автооборачивание всех <select> на странице в кастомный select-wrap
function autoWrapSelects(){
  document.querySelectorAll('select').forEach(function(sel){
    if(sel.closest('.select-wrap')) return; // уже обёрнут
    if(sel.hasAttribute('data-native')) return; // помечен как "не трогать"
    var wrap = document.createElement('div');
    wrap.className = 'select-wrap';
    // Наследуем ширину от select
    var w = sel.style.width || sel.getAttribute('width');
    if(w) wrap.style.width = w;
    else if(sel.offsetWidth) wrap.style.width = sel.offsetWidth + 'px';
    sel.parentNode.insertBefore(wrap, sel);
    wrap.appendChild(sel);
    var box = document.createElement('div');
    box.className = 'sel-box';
    box.innerHTML = '<span class="sel-label"></span><span class="sel-arrow"></span>';
    wrap.appendChild(box);
    var drop = document.createElement('div');
    drop.className = 'sel-dropdown';
    wrap.appendChild(drop);
    initCustomSelect(wrap);
  });
}
autoWrapSelects();


var _mf=null;
function showConfirm(form,msg,btn,cls,title){
  _mf=form;
  document.getElementById('modalMsg').textContent=msg;
  document.getElementById('modalTitle').textContent=title||'Подтвердите действие';
  var b=document.getElementById('modalConfirmBtn');
  b.textContent=btn||'Подтвердить';
  b.className='modal-btn-confirm '+(cls||'danger');
  document.getElementById('confirmModal').classList.add('active');
}
function closeModal(){document.getElementById('confirmModal').classList.remove('active');_mf=null;}
function modalConfirm(){if(_mf){var s=_mf.submit.bind(_mf);_mf.removeAttribute('data-confirm');s();}closeModal();}
(function(){var m=document.getElementById('confirmModal'); if(m){m.addEventListener('click',function(e){if(e.target===this)closeModal();});}})();
document.addEventListener('submit',function(e){
  var f=e.target,msg=f.getAttribute('data-confirm');
  if(msg){e.preventDefault();showConfirm(f,msg,f.getAttribute('data-confirm-btn'),f.getAttribute('data-confirm-class'),f.getAttribute('data-confirm-title'));}
});

// Авто-открытие группы для активной страницы
(function(){
  var activeLink = document.querySelector('.nav-group-items a.active');
  if(activeLink){
    var items = activeLink.closest('.nav-group-items');
    var grp   = activeLink.closest('.nav-group');
    if(items && grp){
      items.classList.add('open');
      var toggle = grp.querySelector('.nav-group-toggle');
      if(toggle) toggle.classList.add('open');
    }
  }
})();
