/* Interaksi bintang di form rating (halaman detail tiket) */
(function () {
    var form = document.querySelector('form[action*="/rating/"]');
    if (!form) return;

    var stars = form.querySelectorAll('input[name="rating"]');
    var labels = form.querySelectorAll('label[for^="star"]');

    function highlightStars(upTo) {
        labels.forEach(function (label) {
            var starId = label.getAttribute('for');
            var starValue = parseInt(starId.replace('star', ''), 10);
            label.style.color = starValue <= upTo ? '#fbbf24' : '#d1d5db';
        });
    }

    function updateStarColors() {
        var checked = form.querySelector('input[name="rating"]:checked');
        if (checked) {
            highlightStars(parseInt(checked.value, 10));
        } else {
            labels.forEach(function (label) {
                label.style.color = '#d1d5db';
            });
        }
    }

    labels.forEach(function (label) {
        label.addEventListener('click', function (e) {
            e.preventDefault();
            var starId = label.getAttribute('for');
            var starInput = document.getElementById(starId);
            if (starInput) {
                starInput.checked = true;
                updateStarColors();
            }
        });
        label.addEventListener('mouseenter', function () {
            var starId = label.getAttribute('for');
            var starValue = parseInt(starId.replace('star', ''), 10);
            highlightStars(starValue);
        });
    });

    form.addEventListener('mouseleave', updateStarColors);
    stars.forEach(function (star) {
        star.addEventListener('change', updateStarColors);
    });
})();
