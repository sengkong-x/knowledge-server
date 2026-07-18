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

      cytoscape({
        container: container,
        elements: elements,
        style: [
          { selector: "node", style: { label: "data(label)" } },
        ],
        layout: { name: "cose" },
      });
    });
});
