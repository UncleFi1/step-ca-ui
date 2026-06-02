(function () {
  function closeOthers(current) {
    var root = current.closest('.mobile-topnav, .admin-topnav');
    if (!root) return;
    root.querySelectorAll('details[open]').forEach(function (item) {
      if (item !== current) item.removeAttribute('open');
    });
  }

  document.addEventListener('toggle', function (event) {
    var item = event.target;
    if (!(item instanceof HTMLDetailsElement) || !item.open) return;
    if (!item.matches('.mobile-dd, .admin-dd')) return;
    closeOthers(item);
  }, true);

  document.addEventListener('click', function (event) {
    if (event.target.closest('.mobile-dd, .admin-dd')) return;
    document.querySelectorAll('.mobile-dd[open], .admin-dd[open]').forEach(function (item) {
      item.removeAttribute('open');
    });
  });

  document.addEventListener('keydown', function (event) {
    if (event.key !== 'Escape') return;
    document.querySelectorAll('.mobile-dd[open], .admin-dd[open]').forEach(function (item) {
      item.removeAttribute('open');
    });
  });
})();
