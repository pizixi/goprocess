(function () {
    const svgAttrs = 'xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"';
    const svg = (body) => `<svg ${svgAttrs}>${body}</svg>`;

    const icons = {
        menu: svg('<path d="M4 6h16"/><path d="M4 12h16"/><path d="M4 18h16"/>'),
        chevronRight: svg('<path d="m9 18 6-6-6-6"/>'),
        home: svg('<path d="M3 11.5 12 4l9 7.5"/><path d="M5 10.5V20h14v-9.5"/><path d="M9.5 20v-5h5v5"/>'),
        processes: svg('<path d="M4 7h16"/><path d="M4 12h16"/><path d="M4 17h16"/><circle cx="8" cy="7" r="1.8"/><circle cx="14" cy="12" r="1.8"/><circle cx="10" cy="17" r="1.8"/>'),
        tasks: svg('<path d="M7 3v4"/><path d="M17 3v4"/><rect x="4" y="5" width="16" height="16" rx="3"/><path d="M4 10h16"/><path d="m8 15 2.2 2.2L16 12"/>'),
        terminal: svg('<path d="m5 7 5 5-5 5"/><path d="M12 19h7"/>'),
        file: svg('<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>'),
        fileText: svg('<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/><path d="M8 13h8"/><path d="M8 17h5"/>'),
        server: svg('<rect x="4" y="4" width="16" height="6" rx="2"/><rect x="4" y="14" width="16" height="6" rx="2"/><path d="M8 7h.01"/><path d="M8 17h.01"/><path d="M12 7h4"/><path d="M12 17h4"/>'),
        upload: svg('<path d="M12 16V4"/><path d="m7 9 5-5 5 5"/><path d="M4 20h16"/>'),
        download: svg('<path d="M12 4v12"/><path d="m7 11 5 5 5-5"/><path d="M4 20h16"/>'),
        folder: svg('<path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>'),
        folderOpen: svg('<path d="M3 8a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v1"/><path d="M3 13h18l-2 6H5z"/>'),
        folderPlus: svg('<path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M12 11v5"/><path d="M9.5 13.5h5"/>'),
        delete: svg('<path d="M4 7h16"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M6 7l1 14h10l1-14"/><path d="M9 7V4h6v3"/>'),
        image: svg('<rect x="4" y="5" width="16" height="14" rx="2"/><circle cx="9" cy="10" r="1.5"/><path d="m4 16 4-4 4 4 2-2 6 6"/>'),
        audio: svg('<path d="M9 18V5l10-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="16" cy="16" r="3"/>'),
        video: svg('<path d="M5 6h11a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2z"/><path d="m18 10 3-2v8l-3-2"/>'),
        pdf: svg('<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/><path d="M8 16h8"/><path d="M8 13h3"/>'),
        word: svg('<path d="M4 4h16v16H4z"/><path d="m7 8 2 8 3-6 3 6 2-8"/>'),
        excel: svg('<path d="M4 4h16v16H4z"/><path d="m8 8 8 8"/><path d="m16 8-8 8"/>'),
        powerpoint: svg('<path d="M4 4h16v16H4z"/><path d="M9 16V8h4a2 2 0 0 1 0 4H9"/>'),
        archive: svg('<path d="M5 7h14v12H5z"/><path d="M7 4h10l2 3H5z"/><path d="M12 10v6"/><path d="M10 12h4"/>'),
        code: svg('<path d="m8 8-4 4 4 4"/><path d="m16 8 4 4-4 4"/><path d="m14 4-4 16"/>'),
        user: svg('<circle cx="12" cy="8" r="4"/><path d="M5 21a7 7 0 0 1 14 0"/>'),
        lock: svg('<rect x="5" y="10" width="14" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/>'),
        search: svg('<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>'),
        close: svg('<path d="M6 6l12 12"/><path d="M18 6 6 18"/>')
    };

    function raw(name) {
        return icons[name] || icons.file || '';
    }

    function icon(name, className) {
        return `<span class="${className || 'svg-icon'}" aria-hidden="true">${raw(name)}</span>`;
    }

    function render(root) {
        (root || document).querySelectorAll('[data-svg-icon]').forEach((el) => {
            el.innerHTML = raw(el.dataset.svgIcon);
            el.setAttribute('aria-hidden', 'true');
        });
    }

    window.gpmIcons = { raw, icon, render, icons };

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => render(document));
    } else {
        render(document);
    }
})();
