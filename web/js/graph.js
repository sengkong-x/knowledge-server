document.addEventListener("DOMContentLoaded", function () {
  var container = document.getElementById("cy");
  if (!container) return;

  fetch(container.dataset.source || "/graph/data")
    .then(function (res) { return res.json(); })
    .then(function (data) {
      var elements = [];
      (data.nodes || []).forEach(function (node) {
        elements.push({ data: { id: node.id, label: node.id } });
        (node.neighbors || []).forEach(function (n) {
          var edgeID = [node.id, n].sort().join("--");
          if (!elements.some(function (el) { return el.data.id === edgeID; })) {
            elements.push({ data: { id: edgeID, source: node.id, target: n } });
          }
        });
      });

      // Cytoscape's style values are plain strings, not CSS — they don't
      // resolve var(--accent) themselves, so the active theme's custom
      // properties are read here and passed in as resolved colors. Without
      // this, node/edge colors would be a fixed hex baked into this file
      // and drift out of sync with whichever theme (light/dark) is active.
      var rootStyle = getComputedStyle(document.documentElement);
      var accentColor = rootStyle.getPropertyValue("--accent").trim();
      var mutedColor = rootStyle.getPropertyValue("--muted").trim();

      cytoscape({
        container: container,
        elements: elements,
        style: [
          {
            selector: "node",
            style: {
              label: "data(label)",
              "font-size": 10,
              "text-max-width": "80px",
              "text-wrap": "ellipsis",
              "text-valign": "bottom",
              "text-margin-y": 4,
              "background-color": accentColor,
              width: 16,
              height: 16,
            },
          },
          {
            selector: "edge",
            style: {
              width: 1,
              "line-color": mutedColor,
              "curve-style": "bezier",
            },
          },
        ],
        layout: { name: "cose", componentSpacing: 120, nodeOverlap: 20, padding: 30 },
      });

      container.classList.add("cy-loaded");
    })
    .catch(function () {
      container.classList.add("cy-loaded", "cy-error");
      container.textContent = "Failed to load graph data.";
    });
});
