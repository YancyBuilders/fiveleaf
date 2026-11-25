(function () {
    const measurer = {
        width(text, size, style) {
            return fiveleafMeasure(String(text), size, style || 0);
        },
    };
    const directions = { TB: 'DOWN', TD: 'DOWN', BT: 'UP', LR: 'RIGHT', RL: 'LEFT' };
    const shapes = new Set([
        'square',
        'rect',
        'rectangle',
        'proc',
        'process',
        'round',
        'rounded',
        'event',
        'diamond',
        'diam',
        'decision',
        'question',
    ]);
    const decisions = new Set(['diamond', 'diam', 'decision', 'question']);
    const laneColors = [
        ['--pele-surface', '--pele-border', '--flow-node-label'],
        ['--flow-teal', '--flow-teal-border', '--flow-teal-label'],
        ['--flow-sage', '--flow-sage-border', '--flow-sage-label'],
    ];
    const sharedColors = ['--flow-shared', '--flow-shared-border', '--flow-shared-label'];
    const edgeStyles = new Set(['stroke', 'stroke-width', 'stroke-dasharray', 'color', 'font-size', 'font-weight']);
    const nodeStyles = new Set([...edgeStyles, 'fill', 'rx', 'ry', 'fill-opacity']);
    const cjk = '\\p{Script=Han}\\p{Script=Hiragana}\\p{Script=Katakana}\\p{Script=Hangul}';
    const wrapTokens = new RegExp(`\\s+|[${cjk}]\\p{Mark}*|[^\\s${cjk}]+`, 'gu');
    let worker;
    let widths = new Map();

    function layout(graph) {
        if (!worker) {
            const scheduled = [];
            const previous = globalThis.setTimeout;
            globalThis.setTimeout = function (fn) {
                scheduled.push(fn);
                return 0;
            };
            try {
                worker = new ELK().worker.worker;
                while (scheduled.length) scheduled.shift()();
            } finally {
                globalThis.setTimeout = previous;
            }
        }
        let response;
        const previous = worker.onmessage;
        worker.onmessage = function (event) {
            response = event.data;
        };
        try {
            worker.dispatcher.saveDispatch({ data: { id: 0, cmd: 'layout', graph } });
        } finally {
            worker.onmessage = previous;
        }
        if (!response || !response.data) throw new Error('No ELK layout');
        return response.data;
    }

    function escape(text) {
        return String(text)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    function entryColors(model) {
        const successors = new Map();
        for (const id of model.nodes.keys()) successors.set(id, []);
        const entries = new Set(successors.keys());
        for (const edge of model.edges) {
            successors.get(edge.start).push(edge.end);
            entries.delete(edge.end);
        }
        const colors = new Map();
        let lane = 0;
        for (const entry of entries) {
            const seen = new Set([entry]);
            for (const id of seen) {
                if (colors.get(id) === sharedColors) continue;
                colors.set(id, colors.has(id) ? sharedColors : laneColors[lane]);
                for (const next of successors.get(id)) seen.add(next);
            }
            lane = (lane + 1) % laneColors.length;
        }
        return colors;
    }

    function styles(model, entity, isEdge) {
        const groups = [];
        if (isEdge) groups.push(model.defaultEdgeStyle || []);
        const classes = entity.classes || [];
        for (const name of isEdge ? classes : ['default'].concat(classes)) {
            const definition = model.classes.get(name);
            if (definition) groups.push(definition.styles || [], definition.textStyles || []);
        }
        groups.push(isEdge ? entity.style || [] : entity.styles || []);
        const result = {};
        for (const group of groups)
            for (const entry of group) {
                const pair = /^\s*([\w-]+)\s*:\s*([^;]+)\s*;?$/.exec(entry);
                if (!pair) return null;
                const key = pair[1],
                    value = pair[2].replace(/\s*!important\s*$/i, '').trim();
                if (isEdge && key === 'fill' && value === 'none') continue;
                if (!(isEdge ? edgeStyles : nodeStyles).has(key) || /[<>"'()\\]/.test(value)) return null;
                if (key === 'font-size') {
                    const size = parseFloat(value);
                    if (!/^(?:\d+(?:\.\d+)?|\.\d+)(?:px)?$/.test(value) || size <= 0 || !Number.isFinite(size)) return null;
                }
                if (key === 'font-weight' && !/^(?:400|600|700|normal|bold)$/.test(value)) return null;
                result[key] = value;
            }
        return result;
    }

    function label(text, size, weight) {
        const widthOf = (value) => {
            const key = `${size}/${weight}/${value}`;
            if (widths.has(key)) return widths.get(key);
            const width = measurer.width(value, size, weight >= 700 ? 1 : weight >= 600 ? 32 : 0);
            if (widths.size < 16384) widths.set(key, width);
            return width;
        };
        const lines = [];
        for (const paragraph of text.split(/<br\s*\/?\s*>|\r?\n/gi)) {
            let line = '';
            for (const token of paragraph.match(wrapTokens) || []) {
                const candidate = (line + token).trimStart();
                if (line.trim() && token.trim() && widthOf(candidate) > 200) {
                    lines.push(line.trimEnd());
                    line = token;
                } else line = candidate;
            }
            lines.push(line.trimEnd());
        }
        return {
            lines,
            size,
            weight,
            width: Math.max(0, ...lines.map(widthOf)),
            height: lines.length * size * 1.16,
        };
    }

    function weightOf(value) {
        if (value === 'normal') return 400;
        if (value === 'bold' || value === 'bolder') return 700;
        return Number(value) || 600;
    }

    function textSVG(label, x, y, color, defaultInk) {
        const top = y - label.height / 2 + label.size * 0.93;
        let paint = defaultInk ? ` fill="var(${defaultInk})"` : '';
        if (color) paint = ` style="fill:${escape(color)}"`;
        return (
            `<text class="pele-label" text-anchor="middle" xml:space="preserve" font-size="${label.size}" font-weight="${label.weight}"${paint}>` +
            label.lines
                .map(
                    (line, index) =>
                        `<tspan x="${x}" y="${top + index * label.size * 1.16}">${escape(line)}</tspan>`,
                )
                .join('') +
            '</text>'
        );
    }

    function point(value) {
        return `${value.x.toFixed(2)},${value.y.toFixed(2)}`;
    }

    function route(points) {
        let path = 'M' + point(points[0]);
        for (let index = 1; index + 1 < points.length; index++) {
            const here = points[index],
                before = points[index - 1],
                after = points[index + 1];
            const incoming = Math.hypot(before.x - here.x, before.y - here.y),
                outgoing = Math.hypot(after.x - here.x, after.y - here.y);
            if (incoming < 0.01 || outgoing < 0.01) continue;
            const radius = Math.min(10, incoming / 2, outgoing / 2);
            const corner = (next, length) =>
                point({
                    x: here.x + ((next.x - here.x) * radius) / length,
                    y: here.y + ((next.y - here.y) * radius) / length,
                });
            path += 'L' + corner(before, incoming) + 'Q' + point(here) + ' ' + corner(after, outgoing);
        }
        return path + 'L' + point(points[points.length - 1]);
    }

    function arrow(points, index, step) {
        const end = points[index],
            next = points[index + step];
        const length = Math.hypot(next.x - end.x, next.y - end.y);
        if (!length) return '';
        const dx = (next.x - end.x) / length,
            dy = (next.y - end.y) / length;
        const offset = Math.min(8, Math.max(0, length / 2 - 4));
        const tip = { x: end.x + dx * offset, y: end.y + dy * offset };
        points[index] = tip;
        const wing = (side) => point({ x: tip.x + 5 * dx + side * 3 * dy, y: tip.y + 5 * dy - side * 3 * dx });
        return `<path class="pele-marker" d="M${wing(1)}L${point(tip)}L${wing(-1)}" fill="none"/>`;
    }

    function flowchart(source) {
        if (/^\s*(?:%%\{|---\s*$|click\s|accTitle\s*:|accDescr\s*:)/im.test(source)) return null;
        const model = Pele.parse(source);
        if (
            !model ||
            model.type !== 'flowchart' ||
            !directions[model.direction] ||
            model.title ||
            model.accTitle ||
            model.accDescr ||
            model.tooltips.size ||
            model.subgraphs.length ||
            model.nodes.size > 160 ||
            model.edges.length > 250
        )
            return null;
        const plain = (text) =>
            !/[<>&`*_\u0000-\u0008]|\\|\$|\bfa[brsld]?:fa-|ﬂ°°?\w+¶ß/i.test(
                text.replace(/<br\s*\/?\s*>/gi, '').replace(/\b\w+(?:_\w+)+\b/g, ''),
            );
        const nodes = [],
            edges = [];
        for (const node of model.nodes.values()) {
            if (
                (node.type && !shapes.has(node.type)) ||
                node.link ||
                (node.classes || []).includes('clickable') ||
                node.labelType === 'markdown' ||
                !plain(node.text || node.id)
            )
                return null;
            const style = styles(model, node, false);
            if (!style) return null;
            const text = label(
                node.text || node.id,
                parseFloat(style['font-size']) || 14,
                weightOf(style['font-weight']),
            );
            nodes.push({
                id: node.id,
                width: text.width + 72,
                height: Math.max(60, text.height + 32),
                text,
                style,
                decision: decisions.has(node.type),
            });
        }
        for (const edge of model.edges) {
            if (
                !['arrow_point', 'double_arrow_point', 'arrow_open'].includes(edge.type) ||
                !['normal', 'dotted', 'thick'].includes(edge.stroke) ||
                edge.length !== 1 ||
                edge.interpolate ||
                edge.labelType === 'markdown' ||
                model.defaultInterpolate ||
                !plain(edge.text || '')
            )
                return null;
            const style = styles(model, edge, true);
            if (!style) return null;
            const text = edge.text
                ? label(edge.text, parseFloat(style['font-size']) || 13, weightOf(style['font-weight']))
                : null;
            edges.push({
                id: edge.id,
                sources: [edge.start],
                targets: [edge.end],
                labels: text
                    ? [
                          {
                              text: edge.text,
                              width: Math.ceil(text.width + 24),
                              height: Math.ceil(Math.max(26, text.height + 8)),
                              layoutOptions: { 'edgeLabels.inline': 'true', 'edgeLabels.placement': 'CENTER' },
                          },
                      ]
                    : [],
                text,
                style,
                edge,
            });
        }
        const graph = layout({
            id: 'root',
            layoutOptions: {
                'elk.algorithm': 'layered',
                'elk.direction': directions[model.direction],
                'elk.hierarchyHandling': 'INCLUDE_CHILDREN',
                'spacing.baseValue': '40',
                'elk.edgeRouting': 'ORTHOGONAL',
                'elk.layered.layering.strategy': 'NETWORK_SIMPLEX',
                'elk.layered.nodePlacement.strategy': 'BRANDES_KOEPF',
                'elk.layered.nodePlacement.bk.fixedAlignment': 'NONE',
                'elk.layered.cycleBreaking.strategy': 'DEPTH_FIRST',
                'elk.layered.considerModelOrder.strategy': 'NODES_AND_EDGES',
                'elk.layered.unnecessaryBendpoints': 'true',
            },
            children: nodes.map(({ id, width, height }) => ({ id, width, height })),
            edges: edges.map(({ id, sources, targets, labels }) => ({ id, sources, targets, labels })),
        });
        const svg = [
            `<svg xmlns="http://www.w3.org/2000/svg" class="fiveleaf-elk pele-flowchart" width="${graph.width}" height="${graph.height}" viewBox="0 0 ${graph.width} ${graph.height}" fill="var(--pele-fg)">`,
        ];
        for (let index = 0; index < edges.length; index++) {
            const spec = edges[index],
                edge = graph.edges[index],
                ink = spec.style.stroke || 'var(--pele-line)';
            if (!edge.sections || edge.sections.length !== 1) return null;
            const section = edge.sections[0],
                points = [section.startPoint].concat(section.bendPoints || [], section.endPoint);
            const dash = spec.style['stroke-dasharray'] || (spec.edge.stroke === 'dotted' ? '3 4' : '');
            let markers = '';
            if (spec.edge.type === 'double_arrow_point') markers += arrow(points, 0, 1);
            if (spec.edge.type !== 'arrow_open') markers += arrow(points, points.length - 1, -1);
            svg.push(
                `<g fill="none" stroke="${escape(ink)}" stroke-width="${escape(spec.style['stroke-width'] || (spec.edge.stroke === 'thick' ? '3' : '1'))}" stroke-linecap="round" stroke-linejoin="round"><path d="${route(points)}"${dash ? ` stroke-dasharray="${escape(dash)}"` : ''}/>`,
            );
            svg.push(markers, '</g>');
        }
        for (let index = 0; index < edges.length; index++) {
            const spec = edges[index];
            if (!spec.text) continue;
            const box = graph.edges[index].labels[0];
            svg.push(
                `<g class="pele-edge-label"><rect x="${box.x}" y="${box.y}" width="${box.width}" height="${box.height}" rx="13"/>`,
                textSVG(spec.text, box.x + box.width / 2, box.y + box.height / 2, spec.style.color),
                '</g>',
            );
        }
        const colors = entryColors(model);
        for (let index = 0; index < nodes.length; index++) {
            const spec = nodes[index],
                node = graph.children[index],
                style = spec.style;
            const [fill, border, ink] = colors.get(node.id) || laneColors[0];
            const defaultInk = style.fill || spec.decision ? undefined : ink;
            const shapeStyle = Object.keys(style)
                .filter((key) => !['color', 'font-size', 'font-weight'].includes(key))
                .map((key) => `${key}:${style[key]}`)
                .join(';');
            svg.push(
                `<g class="pele-node ${spec.decision ? 'pele-shape-diam' : 'pele-shape-rect'}" data-id="${escape(node.id)}"><rect x="${node.x}" y="${node.y}" width="${node.width}" height="${node.height}" rx="16" fill="var(${fill})" stroke="var(${border})"${shapeStyle ? ` style="${escape(shapeStyle)}"` : ''}/>`,
                textSVG(spec.text, node.x + node.width / 2, node.y + node.height / 2, style.color, defaultInk),
                '</g>',
            );
        }
        return svg.join('') + '</svg>';
    }

    return function (source, fresh) {
        if (fresh) {
            Pele.forgetTextWidths();
            widths.clear();
        }
        if (/^\s*(?:flowchart|graph)\b/im.test(source)) {
            try {
                const svg = flowchart(source);
                if (svg) return svg;
            } catch (error) {}
        }
        return Pele.render(source, { measurer, responsive: false, autoDirection: false }).svg;
    };
})();
