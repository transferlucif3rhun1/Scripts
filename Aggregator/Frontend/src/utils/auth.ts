export function isLoggedIn() {
  return !!localStorage.getItem("token");
}

export function logout() {
  localStorage.removeItem("token");
  window.location.href = "/login";
}
