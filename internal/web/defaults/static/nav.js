(function ($) {
  'use strict';

  function collapseNavOnMobile() {
    var nav = document.getElementById('mainNav');
    if (!nav || !nav.classList.contains('show')) {
      return;
    }
    var collapse = bootstrap.Collapse.getInstance(nav);
    if (collapse) {
      collapse.hide();
    }
  }

  function highlightActiveNav() {
    var path = window.location.pathname;
    $('#mainNav .nav-link[data-nav]').each(function () {
      var href = $(this).attr('data-nav');
      if (!href) {
        return;
      }
      if (href === path || (href !== '/menu' && path.indexOf(href) === 0)) {
        $(this).addClass('active');
      }
    });
  }

  $(function () {
    $('#lang-next').val(window.location.pathname + window.location.search);

    $('.lang-select').on('change', function () {
      $('#lang-next').val(window.location.pathname + window.location.search);
      $(this).closest('form').trigger('submit');
    });

    $('#mainNav .nav-link[data-nav]').on('click', function () {
      if (window.matchMedia('(max-width: 991.98px)').matches) {
        collapseNavOnMobile();
      }
    });

    highlightActiveNav();
  });
})(jQuery);
