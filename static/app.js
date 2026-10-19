const $ = (id) => document.getElementById(id);

function show(id) {
  document.querySelectorAll(".page").forEach((page) => {
    page.classList.remove("active");
  });

  $(id).classList.add("active");
}

function toast(message) {
  const box = $("toast");

  box.innerText = message;
  box.style.display = "block";

  setTimeout(() => {
    box.style.display = "none";
  }, 2500);
}

async function request(url, options) {
  const response = await fetch(url, options);

  if (!response.ok) {
    throw new Error(await response.text());
  }

  return response.json();
}

async function loadDashboard() {
  const state = await request("/api/state");

  $("campaigns").innerText =
    state.campaigns.length + " campaigns";

  $("recipients").innerText =
    state.contacts.length + " recipients";

  const totalSent = state.campaigns.reduce(
    (total, campaign) => total + campaign.Sent,
    0
  );

  $("sent").innerText = totalSent + " sent";

  const rows = state.campaigns
    .map(
      (campaign) => `
        <tr>
          <td>${campaign.Name}</td>
          <td>${campaign.Sent}</td>
          <td>${campaign.Failed}</td>
        </tr>
      `
    )
    .join("");

  $("history").innerHTML = `
    <h3>Campaign History</h3>

    <table>
      <tr>
        <th>Name</th>
        <th>Sent</th>
        <th>Failed</th>
      </tr>

      ${rows}
    </table>
  `;
}

async function upload() {
  const file = $("csv").files[0];

  if (!file) {
    toast("Please choose a CSV file");
    return;
  }

  const form = new FormData();
  form.append("file", file);

  try {
    const result
