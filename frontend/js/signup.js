const form = document.getElementById('sign-up');

form.addEventListener('submit', (event) => {
  event.preventDefault();
  const confirmPassword = document.getElementById('confirm-password');
  const password = document.getElementById('password');

  if (confirmPassword.value !== password.value) {
    confirmPassword.setCustomValidity('Confirm password must match password.');
  } else {
    confirmPassword.setCustomValidity('');
  }

  if (form.reportValidity()) {
    form.submit();
  } else {
    confirmPassword.setCustomValidity('');
  }
});