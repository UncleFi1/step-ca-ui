/* ─────────────────────────────────────────────
 * Кастомный datepicker (без зависимостей)
 *
 * Использование в HTML:
 *   <div class="dp-wrap" data-datepicker data-name="custom_datetime"
 *        data-min="now" data-placeholder="Выберите дату и время">
 *   </div>
 *
 * Атрибуты:
 *   data-name        — name скрытого поля (отправляется в форму)
 *   data-min         — "now" или ISO-строка минимально допустимой даты
 *   data-placeholder — текст-плейсхолдер
 *
 * Формат отправляемого значения: "YYYY-MM-DD HH:MM"
 * Пустое значение = поле не отправляется.
 *
 * Валидация: если дата выбрана, время обязательно (часы И минуты должны
 * быть введены).
 * ───────────────────────────────────────────── */
(function () {
  'use strict';

  const MONTHS = ['Январь','Февраль','Март','Апрель','Май','Июнь',
                  'Июль','Август','Сентябрь','Октябрь','Ноябрь','Декабрь'];
  const WEEKDAYS = ['пн','вт','ср','чт','пт','сб','вс'];

  function pad(n) { return String(n).padStart(2, '0'); }

  function fmtDateHuman(y, m, d, hh, mm) {
    return `${pad(d)}.${pad(m+1)}.${y} ${pad(hh)}:${pad(mm)}`;
  }
  function fmtDateISO(y, m, d, hh, mm) {
    return `${y}-${pad(m+1)}-${pad(d)} ${pad(hh)}:${pad(mm)}`;
  }
  function sameDay(a, b) {
    return a.getFullYear() === b.getFullYear()
        && a.getMonth() === b.getMonth()
        && a.getDate() === b.getDate();
  }

  function buildPicker(wrap) {
    if (wrap.dataset.dpInit === '1') return;
    wrap.dataset.dpInit = '1';

    const name        = wrap.dataset.name || 'custom_datetime';
    const placeholder = wrap.dataset.placeholder || 'Выберите дату и время';
    const minMode     = wrap.dataset.min || ''; // "now" — запрещает прошлое

    // Состояние
    let viewYear, viewMonth;
    let pickedDate = null;       // {y, m, d} или null
    let pickedHH = null, pickedMM = null;

    const today = new Date();
    viewYear = today.getFullYear();
    viewMonth = today.getMonth();

    // ── DOM ───────────────────────────────────────
    wrap.innerHTML = `
      <div class="dp-input dp-empty" tabindex="0">${placeholder}</div>
      <svg class="dp-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor"
           stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <rect x="3" y="4" width="18" height="18" rx="2" ry="2"></rect>
        <line x1="16" y1="2" x2="16" y2="6"></line>
        <line x1="8"  y1="2" x2="8"  y2="6"></line>
        <line x1="3" y1="10" x2="21" y2="10"></line>
      </svg>
      <input type="hidden" name="${name}" value="">
      <div class="dp-pop">
        <div class="dp-head">
          <button type="button" class="dp-nav dp-prev" aria-label="Предыдущий месяц">‹</button>
          <div class="dp-title"></div>
          <button type="button" class="dp-nav dp-next" aria-label="Следующий месяц">›</button>
        </div>
        <div class="dp-weekdays">
          ${WEEKDAYS.map(w => `<div class="dp-weekday">${w}</div>`).join('')}
        </div>
        <div class="dp-grid"></div>
        <div class="dp-time">
          <span class="dp-time-label">Время:</span>
          <input type="number" class="dp-hh" min="0" max="23" placeholder="чч" inputmode="numeric">
          <span class="dp-time-sep">:</span>
          <input type="number" class="dp-mm" min="0" max="59" placeholder="мм" inputmode="numeric">
        </div>
        <div class="dp-foot">
          <button type="button" class="dp-clear">Очистить</button>
          <button type="button" class="dp-ok">Готово</button>
        </div>
      </div>
      <div class="dp-error">Укажите часы и минуты</div>
    `;

    const inputDisplay = wrap.querySelector('.dp-input');
    const hidden       = wrap.querySelector('input[type="hidden"]');
    const pop          = wrap.querySelector('.dp-pop');
    const title        = wrap.querySelector('.dp-title');
    const grid         = wrap.querySelector('.dp-grid');
    const btnPrev      = wrap.querySelector('.dp-prev');
    const btnNext      = wrap.querySelector('.dp-next');
    const inHH         = wrap.querySelector('.dp-hh');
    const inMM         = wrap.querySelector('.dp-mm');
    const btnClear     = wrap.querySelector('.dp-clear');
    const btnOk        = wrap.querySelector('.dp-ok');

    function clearError() { wrap.classList.remove('dp-has-error'); }
    function showError()  { wrap.classList.add('dp-has-error'); }

    function isDateDisabled(y, m, d) {
      if (minMode !== 'now') return false;
      const t = new Date();
      const candidate = new Date(y, m, d, 23, 59, 59);
      return candidate < new Date(t.getFullYear(), t.getMonth(), t.getDate(), 0, 0, 0);
    }

    function renderGrid() {
      title.textContent = `${MONTHS[viewMonth]} ${viewYear}`;
      grid.innerHTML = '';

      // Первый день месяца + сдвиг до понедельника
      const first = new Date(viewYear, viewMonth, 1);
      // getDay(): 0=вс..6=сб → переведём в 0=пн..6=вс
      let offset = (first.getDay() + 6) % 7;

      const daysInMonth = new Date(viewYear, viewMonth + 1, 0).getDate();
      const prevMonthDays = new Date(viewYear, viewMonth, 0).getDate();

      const cells = [];
      // Хвост предыдущего месяца
      for (let i = offset - 1; i >= 0; i--) {
        const d = prevMonthDays - i;
        const m = viewMonth === 0 ? 11 : viewMonth - 1;
        const y = viewMonth === 0 ? viewYear - 1 : viewYear;
        cells.push({y, m, d, other: true});
      }
      // Текущий месяц
      for (let d = 1; d <= daysInMonth; d++) {
        cells.push({y: viewYear, m: viewMonth, d, other: false});
      }
      // Голова следующего месяца до 42 ячеек (6×7)
      while (cells.length < 42) {
        const last = cells[cells.length - 1];
        const next = new Date(last.y, last.m, last.d + 1);
        cells.push({y: next.getFullYear(), m: next.getMonth(), d: next.getDate(), other: true});
      }

      const today = new Date();
      cells.forEach(c => {
        const btn = document.createElement('div');
        btn.className = 'dp-day';
        btn.textContent = c.d;
        if (c.other) btn.classList.add('dp-other');
        if (sameDay(new Date(c.y, c.m, c.d), today)) btn.classList.add('dp-today');
        if (pickedDate && pickedDate.y === c.y && pickedDate.m === c.m && pickedDate.d === c.d) {
          btn.classList.add('dp-selected');
        }
        if (isDateDisabled(c.y, c.m, c.d)) {
          btn.classList.add('dp-disabled');
        } else {
          btn.addEventListener('click', () => {
            pickedDate = {y: c.y, m: c.m, d: c.d};
            // если день выпадает за текущий месяц — переключим вид
            if (c.m !== viewMonth) {
              viewYear = c.y; viewMonth = c.m;
            }
            renderGrid();
            clearError();
            inHH.focus();
          });
        }
        grid.appendChild(btn);
      });
    }

    function commit() {
      // Валидация: если есть дата — нужны и часы, и минуты
      if (pickedDate) {
        const hhRaw = inHH.value.trim();
        const mmRaw = inMM.value.trim();
        if (hhRaw === '' || mmRaw === '') {
          showError();
          return false;
        }
        const hh = parseInt(hhRaw, 10);
        const mm = parseInt(mmRaw, 10);
        if (isNaN(hh) || hh < 0 || hh > 23 || isNaN(mm) || mm < 0 || mm > 59) {
          showError();
          return false;
        }
        pickedHH = hh;
        pickedMM = mm;
        hidden.value = fmtDateISO(pickedDate.y, pickedDate.m, pickedDate.d, hh, mm);
        inputDisplay.textContent = fmtDateHuman(pickedDate.y, pickedDate.m, pickedDate.d, hh, mm);
        inputDisplay.classList.remove('dp-empty');
      } else {
        hidden.value = '';
        inputDisplay.textContent = placeholder;
        inputDisplay.classList.add('dp-empty');
      }
      clearError();
      close();
      return true;
    }

    function clearAll() {
      pickedDate = null;
      pickedHH = null; pickedMM = null;
      inHH.value = ''; inMM.value = '';
      hidden.value = '';
      inputDisplay.textContent = placeholder;
      inputDisplay.classList.add('dp-empty');
      clearError();
      renderGrid();
    }

    function open()  { pop.classList.add('open');  }
    function close() { pop.classList.remove('open'); }
    function toggle() { pop.classList.contains('open') ? close() : open(); }

    // События
    inputDisplay.addEventListener('click', toggle);
    inputDisplay.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggle(); }
    });

    btnPrev.addEventListener('click', () => {
      if (viewMonth === 0) { viewMonth = 11; viewYear--; }
      else viewMonth--;
      renderGrid();
    });
    btnNext.addEventListener('click', () => {
      if (viewMonth === 11) { viewMonth = 0; viewYear++; }
      else viewMonth++;
      renderGrid();
    });

    btnOk.addEventListener('click', commit);
    btnClear.addEventListener('click', clearAll);

    // Esc закрывает
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape' && pop.classList.contains('open')) close();
    });

    // Клик вне — закрывает
    document.addEventListener('click', (e) => {
      if (!wrap.contains(e.target) && pop.classList.contains('open')) {
        close();
      }
    });

    // Если пользователь меняет часы/минуты — снимаем подсветку ошибки
    inHH.addEventListener('input', clearError);
    inMM.addEventListener('input', clearError);

    // При сабмите формы валидируем и подтверждаем
    const form = wrap.closest('form');
    if (form) {
      form.addEventListener('submit', (e) => {
        // Если дата выбрана, но окно закрыто — нужно сделать commit чтобы записать в hidden
        // Если поповер открыт — пробуем закоммитить, ловим валидацию
        if (pickedDate && (inHH.value.trim() === '' || inMM.value.trim() === '')) {
          e.preventDefault();
          open();
          showError();
          inHH.focus();
          return false;
        }
        // Если поповер открыт — закоммитим (он переустановит hidden)
        if (pop.classList.contains('open')) {
          if (!commit()) {
            e.preventDefault();
            return false;
          }
        }
      });
    }

    // Первичная отрисовка
    renderGrid();
  }

  function initAll() {
    document.querySelectorAll('.dp-wrap[data-datepicker]').forEach(buildPicker);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initAll);
  } else {
    initAll();
  }

  // Экспорт на случай динамической вставки
  window.initDatepickers = initAll;
})();
