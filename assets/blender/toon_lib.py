# Kammzeit Toon-Bibliothek fuer Blender 5.x (bpy) - gemeinsamer Look fuer alle Illustrationen.
#
# KEIN EEVEE (Mesa/radeonsi-Absturz beim Shader-Kompilieren). Toon-Look komplett als
# Emission-Shader: Normale . Lichtrichtung -> ColorRamp "Constant" (2-3 Stufen je Palettenfarbe)
# -> Emission. Kontur in Nachtblau per Inverted Hull (Solidify nach aussen, Backfacing-Mix).
# Gerendert wird in Cycles CPU mit wenigen Samples (nur Kantenglaettung, es gibt kein Rauschen).
# Viewport nur Solid (Workbench), luxend sieht den Aufbau trotzdem.
#
# Benutzung im eigenen Skript:
#   import sys; sys.path.insert(0, "<dieser Ordner>"); import toon_lib as T
#   def build():
#       T.set_light(az=-130, el=48)
#       body = T.ellipsoid("Ball", (0, 0, 0.3), (0.3, 0.3, 0.3), color="koralle", steps=3)
#       yield                       # jedes yield = ein sichtbarer Schritt
#       k = T.append_kammi(loc=(0.6, 0, 0), rot_z=-20, pose="winkt")
#       yield
#   T.Run("name", build, res=(1200, 900), out="/pfad/bild.png", outline_px=2.2).start()
# Start (sichtbar, ein Blender gleichzeitig):
#   flock /tmp/kammzeit-blender.lock nice -n 19 ionice -c3 blender --factory-startup -t 6 --python skript.py
#
# Einheiten: Meter. Menschen ~1.65-1.75 m, Kammi ~0.62 m. Blickrichtung der Kamera Standard: von -Y
# (az=-90), Figuren schauen also nach -Y. Boden = z 0, NICHTS schwebt.
import bpy, bmesh, math, os, sys, time, traceback
from mathutils import Vector, Matrix, Quaternion, Euler
from bpy_extras.object_utils import world_to_camera_view

HERE = os.path.dirname(os.path.abspath(__file__))
ASSETS = "/tmp/claude-1000/-home-luxend/392722f8-b013-47c4-8bce-99419030ab5b/scratchpad/kammzeit-assets"
KAMMI_BLEND = os.path.join(ASSETS, "blender", "kammi.blend")
STEP = 0.5          # Sekunden pro sichtbarem Schritt
SCENE = bpy.context.scene
STATE = {"orbit": True, "target": Vector((0, 0, 0.6)), "size": 1.5, "light": (-130.0, 48.0)}


def log(*a):
    print("[toon]", *a, flush=True)


# ================================================================ Palette

PAL = {
    "himmel_hell": "#E3F3FF",
    "himmel": "#7CC4FF",
    "blau": "#1E6BFF",
    "gelb": "#FFC93C",
    "koralle": "#FF7A59",
    "mint": "#3CCF91",
    "nachtblau": "#13294B",
    "weiss": "#FFFFFF",
}


def _rgb(h):
    h = PAL.get(h, h).lstrip("#")
    return [int(h[i:i + 2], 16) / 255 for i in (0, 2, 4)]


def _hex(c):
    return "#%02X%02X%02X" % tuple(max(0, min(255, round(x * 255))) for x in c)


def mix(a, b, t):
    """Mischung zweier Farben (Palettenname oder Hex), t=0 -> a, t=1 -> b."""
    ca, cb = _rgb(a), _rgb(b)
    return _hex([x + (y - x) * t for x, y in zip(ca, cb)])


# Abstufungen der Palette (erlaubt laut Vertrag): Haut, Haare, Neutraltoene
PAL.update({
    "haut_hell": mix(mix("koralle", "gelb", 0.5), "weiss", 0.64),
    "haut_mittel": mix(mix("koralle", "gelb", 0.45), "nachtblau", 0.26),
    "haut_dunkel": mix(mix("koralle", "gelb", 0.40), "nachtblau", 0.56),
    "creme": mix("gelb", "weiss", 0.82),
    "hellgrau": mix("nachtblau", "weiss", 0.86),
    "grau": mix("nachtblau", "weiss", 0.62),
    "mittelblau": mix("blau", "himmel", 0.5),
    "rosa": mix("koralle", "weiss", 0.55),
    "mint_hell": mix("mint", "weiss", 0.55),
    "gelb_hell": mix("gelb", "weiss", 0.5),
    "haar_dunkel": mix("nachtblau", "koralle", 0.12),
    "haar_braun": mix(mix("koralle", "gelb", 0.35), "nachtblau", 0.62),
    "haar_blond": mix("gelb", "koralle", 0.15),
    "haar_rot": mix("koralle", "nachtblau", 0.18),
})


def lin(h, a=1.0):
    c = _rgb(h)
    l = [x / 12.92 if x <= 0.04045 else ((x + 0.055) / 1.055) ** 2.4 for x in c]
    return (l[0], l[1], l[2], a)


def _lum(h):
    r, g, b = _rgb(h)
    return 0.2126 * r + 0.7152 * g + 0.0722 * b


def shade(h):
    """Schattenton: Richtung Nachtblau (helle Farben weniger, damit Weiss nicht grau wird)."""
    return mix(h, "nachtblau", 0.13 if _lum(h) > 0.85 else 0.26)


def light(h):
    """Lichtton fuer 3-stufiges Toon."""
    return mix(h, "weiss", 0.30)


# ================================================================ Materialien

_MATS = {}
LIGHT_NODE = "KZ_Licht"


def _light_vec():
    az, el = (math.radians(x) for x in STATE["light"])
    return (math.cos(az) * math.cos(el), math.sin(az) * math.cos(el), math.sin(el))


def set_light(az=-130.0, el=48.0):
    """Lichtrichtung (von wo das Licht kommt) fuer ALLE Toon-Materialien, auch angehaengte (Kammi).
    az=-130 = vorne links (Kamera bei az -90), el = Hoehe in Grad."""
    STATE["light"] = (az, el)
    v = _light_vec()
    for m in bpy.data.materials:
        if m.node_tree and LIGHT_NODE in m.node_tree.nodes:
            n = m.node_tree.nodes[LIGHT_NODE]
            n.inputs[0].default_value, n.inputs[1].default_value, n.inputs[2].default_value = v


def toon(color, steps=2, name=None, t0=0.40, t1=0.86, shadow=None, hi=None):
    """Flaches Toon-Material. color = Palettenname oder Hex. steps 2 = Schatten+Grundton,
    3 = zusaetzlich Lichtkante. t0/t1 = Schwellen auf (N.L+1)/2."""
    key = name or "toon_%s_%d" % (color, steps)
    if key in _MATS and _MATS[key].name in bpy.data.materials:
        return _MATS[key]
    base = PAL.get(color, color)
    m = bpy.data.materials.new(key)
    try:
        m.use_nodes = True
    except Exception:
        pass
    nt = m.node_tree
    for n in list(nt.nodes):
        nt.nodes.remove(n)
    out = nt.nodes.new("ShaderNodeOutputMaterial")
    geo = nt.nodes.new("ShaderNodeNewGeometry")
    lv = nt.nodes.new("ShaderNodeCombineXYZ")
    lv.name = LIGHT_NODE
    lv.label = "Lichtrichtung"
    v = _light_vec()
    lv.inputs[0].default_value, lv.inputs[1].default_value, lv.inputs[2].default_value = v
    dot = nt.nodes.new("ShaderNodeVectorMath")
    dot.operation = "DOT_PRODUCT"
    ma = nt.nodes.new("ShaderNodeMath")
    ma.operation = "MULTIPLY_ADD"
    ma.inputs[1].default_value = 0.5
    ma.inputs[2].default_value = 0.5
    ramp = nt.nodes.new("ShaderNodeValToRGB")
    cr = ramp.color_ramp
    cr.interpolation = "CONSTANT"
    cr.elements[0].position = 0.0
    cr.elements[0].color = lin(shadow or shade(base))
    cr.elements[1].position = t0
    cr.elements[1].color = lin(base)
    if steps >= 3:
        e = cr.elements.new(t1)
        e.color = lin(hi or light(base))
    em = nt.nodes.new("ShaderNodeEmission")
    em.inputs["Strength"].default_value = 1.0
    nt.links.new(geo.outputs["Normal"], dot.inputs[0])
    nt.links.new(lv.outputs[0], dot.inputs[1])
    nt.links.new(dot.outputs["Value"], ma.inputs[0])
    nt.links.new(ma.outputs[0], ramp.inputs["Fac"])
    nt.links.new(ramp.outputs["Color"], em.inputs["Color"])
    nt.links.new(em.outputs[0], out.inputs["Surface"])
    for i, n in enumerate((geo, lv, dot, ma, ramp, em, out)):
        n.location = (i * 220 - 600, 0)
    m.diffuse_color = lin(base)          # Solid-Viewport zeigt den Grundton
    m.roughness = 0.8
    _MATS[key] = m
    return m


def flat(color, name=None):
    """Einfarbig ohne Schattierung (Augen, Linien, Grafik)."""
    key = name or "flach_%s" % color
    if key in _MATS and _MATS[key].name in bpy.data.materials:
        return _MATS[key]
    base = PAL.get(color, color)
    return toon(color, steps=2, name=key, shadow=base)


def outline_mat(color="nachtblau"):
    key = "kontur_%s" % color
    if key in _MATS and _MATS[key].name in bpy.data.materials:
        return _MATS[key]
    m = bpy.data.materials.new(key)
    try:
        m.use_nodes = True
    except Exception:
        pass
    nt = m.node_tree
    for n in list(nt.nodes):
        nt.nodes.remove(n)
    out = nt.nodes.new("ShaderNodeOutputMaterial")
    geo = nt.nodes.new("ShaderNodeNewGeometry")
    em = nt.nodes.new("ShaderNodeEmission")
    em.inputs["Color"].default_value = lin(color)
    tr = nt.nodes.new("ShaderNodeBsdfTransparent")
    mx = nt.nodes.new("ShaderNodeMixShader")
    nt.links.new(geo.outputs["Backfacing"], mx.inputs["Fac"])
    nt.links.new(em.outputs[0], mx.inputs[1])
    nt.links.new(tr.outputs[0], mx.inputs[2])
    nt.links.new(mx.outputs[0], out.inputs["Surface"])
    m.diffuse_color = lin(color)
    # Achtung: Cycles 5.x beachtet use_backface_culling und wuerde den sichtbaren Teil der Huelle
    # wegschneiden. Daher aus; im Solid-Viewport sorgt shading.show_backface_culling fuer den Rand.
    m.use_backface_culling = False
    _MATS[key] = m
    return m


def shadow_mat(alpha=0.12, color="nachtblau"):
    key = "schatten_%s_%.3f" % (color, alpha)
    if key in _MATS and _MATS[key].name in bpy.data.materials:
        return _MATS[key]
    m = bpy.data.materials.new(key)
    try:
        m.use_nodes = True
    except Exception:
        pass
    nt = m.node_tree
    for n in list(nt.nodes):
        nt.nodes.remove(n)
    out = nt.nodes.new("ShaderNodeOutputMaterial")
    em = nt.nodes.new("ShaderNodeEmission")
    em.inputs["Color"].default_value = lin(color)
    tr = nt.nodes.new("ShaderNodeBsdfTransparent")
    mx = nt.nodes.new("ShaderNodeMixShader")
    mx.inputs["Fac"].default_value = alpha
    nt.links.new(tr.outputs[0], mx.inputs[1])
    nt.links.new(em.outputs[0], mx.inputs[2])
    nt.links.new(mx.outputs[0], out.inputs["Surface"])
    m.diffuse_color = lin(mix(color, "weiss", 0.8), 1.0)
    _MATS[key] = m
    return m


def _mat(color, steps):
    if isinstance(color, bpy.types.Material):
        return color
    return toon(color, steps)


# ================================================================ Objekte

def link(obj, parent=None, keep_world=False):
    SCENE.collection.objects.link(obj)
    if parent is not None:
        mw = obj.matrix_world.copy()
        obj.parent = parent
        if keep_world:
            obj.matrix_parent_inverse = parent.matrix_world.inverted()
    return obj


def empty(name, loc=(0, 0, 0), rot=(0, 0, 0), parent=None, size=0.05):
    e = bpy.data.objects.new(name, None)
    e.empty_display_size = size
    e.location = loc
    e.rotation_euler = [math.radians(a) for a in rot]
    return link(e, parent)


def mesh_obj(name, bm, color=None, steps=2, parent=None, smooth=True, loc=None, rot=None):
    me = bpy.data.meshes.new(name)
    bm.to_mesh(me)
    bm.free()
    if smooth:
        me.shade_smooth()
    o = bpy.data.objects.new(name, me)
    if loc is not None:
        o.location = loc
    if rot is not None:
        o.rotation_euler = [math.radians(a) for a in rot]
    link(o, parent)
    if color is not None:
        me.materials.append(_mat(color, steps))
    return o


def bevel(o, width, seg=3, angle=35):
    b = o.modifiers.new("Bevel", "BEVEL")
    b.width = width
    b.segments = seg
    b.limit_method = "ANGLE"
    b.angle_limit = math.radians(angle)
    return b


def ellipsoid(name, loc, radii, color=None, steps=2, rot=(0, 0, 0), parent=None, seg=40, rings=24):
    """Ellipsoid, Mitte loc (lokal zum parent), radii (rx, ry, rz), rot in Grad."""
    bm = bmesh.new()
    bmesh.ops.create_uvsphere(bm, u_segments=seg, v_segments=rings, radius=1.0)
    for v in bm.verts:
        v.co = Vector((v.co.x * radii[0], v.co.y * radii[1], v.co.z * radii[2]))
    return mesh_obj(name, bm, color, steps, parent, loc=loc, rot=rot)


def revolve(name, profile, color=None, steps=2, loc=(0, 0, 0), rot=(0, 0, 0), parent=None, seg=48, sy=1.0):
    """Drehkoerper um die lokale Z-Achse. profile = [(z, r), ...] von unten nach oben;
    r=0 am Anfang/Ende schliesst den Koerper. sy staucht in Y (ovale Querschnitte)."""
    bm = bmesh.new()
    rings = []
    for z, r in profile:
        if r <= 1e-6:
            rings.append([bm.verts.new((0, 0, z))])
        else:
            rings.append([bm.verts.new((r * math.cos(2 * math.pi * k / seg), sy * r * math.sin(2 * math.pi * k / seg), z))
                          for k in range(seg)])
    for a, b in zip(rings, rings[1:]):
        if len(a) == 1 and len(b) == 1:
            continue
        for k in range(seg):
            j = (k + 1) % seg
            if len(a) == 1:
                bm.faces.new((a[0], b[j], b[k]))
            elif len(b) == 1:
                bm.faces.new((a[k], a[j], b[0]))
            else:
                bm.faces.new((a[k], a[j], b[j], b[k]))
    if len(rings[0]) > 1:
        bm.faces.new(list(reversed(rings[0])))
    if len(rings[-1]) > 1:
        bm.faces.new(rings[-1])
    bmesh.ops.recalc_face_normals(bm, faces=bm.faces[:])
    return mesh_obj(name, bm, color, steps, parent, loc=loc, rot=rot)


def _frame(t):
    t = t.normalized()
    up = Vector((0, 0, 1)) if abs(t.z) < 0.95 else Vector((1, 0, 0))
    s = t.cross(up).normalized()
    n = s.cross(t).normalized()
    return s, n


def tube(name, pts, radius, color=None, steps=2, parent=None, seg=20, caps=True, radii=None):
    """Rohr entlang einer Punktliste (lokal), runde Enden. radii = Radius je Punkt (optional)."""
    pts = [Vector(p) for p in pts]
    rr = radii or [radius] * len(pts)
    bm = bmesh.new()
    rings = []
    s_prev = None
    for i, p in enumerate(pts):
        t = (pts[min(i + 1, len(pts) - 1)] - pts[max(i - 1, 0)])
        s, n = _frame(t)
        if s_prev is not None and s.dot(s_prev) < 0:
            s, n = -s, -n
        s_prev = s
        rings.append([bm.verts.new(p + (s * math.cos(2 * math.pi * k / seg) + n * math.sin(2 * math.pi * k / seg)) * rr[i])
                      for k in range(seg)])
    for a, b in zip(rings, rings[1:]):
        for k in range(seg):
            j = (k + 1) % seg
            bm.faces.new((a[k], a[j], b[j], b[k]))
    if caps:
        for idx, sign in ((0, -1), (-1, 1)):
            p = pts[idx]
            t = (pts[1] - pts[0]) if idx == 0 else (pts[-1] - pts[-2])
            t = t.normalized() * sign
            ring = rings[idx]
            prev = ring
            for k in range(1, 5):
                a = math.pi / 2 * k / 5
                c = p + t * rr[idx] * math.sin(a)
                ring2 = [bm.verts.new(c + (v.co - p) * math.cos(a)) for v in ring]
                for q in range(seg):
                    j = (q + 1) % seg
                    if sign > 0:
                        bm.faces.new((prev[q], prev[j], ring2[j], ring2[q]))
                    else:
                        bm.faces.new((prev[j], prev[q], ring2[q], ring2[j]))
                prev = ring2
            tip = bm.verts.new(p + t * rr[idx])
            for q in range(seg):
                j = (q + 1) % seg
                if sign > 0:
                    bm.faces.new((prev[q], prev[j], tip))
                else:
                    bm.faces.new((prev[j], prev[q], tip))
    bmesh.ops.recalc_face_normals(bm, faces=bm.faces[:])
    return mesh_obj(name, bm, color, steps, parent)


def capsule(name, p0, p1, r0, r1=None, color=None, steps=2, parent=None, seg=24):
    """Kapsel von p0 nach p1 (lokal), Radius r0 -> r1 (Arme, Beine, Finger)."""
    r1 = r0 if r1 is None else r1
    p0, p1 = Vector(p0), Vector(p1)
    n = 6
    pts = [p0.lerp(p1, i / n) for i in range(n + 1)]
    rad = [r0 + (r1 - r0) * i / n for i in range(n + 1)]
    return tube(name, pts, r0, color, steps, parent, seg=seg, radii=rad)


def rbox(name, size, loc=(0, 0, 0), radius=0.02, color=None, steps=2, rot=(0, 0, 0), parent=None, seg=4):
    """Quader (Kantenlaengen size) mit gerundeten Kanten, Mitte loc."""
    bm = bmesh.new()
    bmesh.ops.create_cube(bm, size=1.0)
    for v in bm.verts:
        v.co = Vector((v.co.x * size[0], v.co.y * size[1], v.co.z * size[2]))
    o = mesh_obj(name, bm, color, steps, parent, smooth=True, loc=loc, rot=rot)
    if radius > 0:
        b = o.modifiers.new("Bevel", "BEVEL")
        b.width = radius
        b.segments = seg
        b.limit_method = "NONE"
        try:
            b.harden_normals = False
        except Exception:
            pass
    return o


def cylinder(name, loc, r, h, color=None, steps=2, rot=(0, 0, 0), parent=None, seg=48, radius=0.0):
    """Zylinder, unten auf loc (z), Hoehe h."""
    bm = bmesh.new()
    bmesh.ops.create_cone(bm, cap_ends=True, cap_tris=False, segments=seg, radius1=r, radius2=r, depth=h,
                          matrix=Matrix.Translation((0, 0, h / 2)))
    o = mesh_obj(name, bm, color, steps, parent, loc=loc, rot=rot)
    if radius > 0:
        bevel(o, radius, 3, 40)
    return o


def loft(name, stations, color=None, steps=2, parent=None, n=40, loc=(0, 0, 0), rot=(0, 0, 0)):
    """Huelle entlang lokaler X-Achse. stations: (x, ry, rz[, exp[, zoff]]) Superellipsen (exp 2 = Ellipse)."""
    bm = bmesh.new()
    rings = []
    for st in stations:
        x, ry, rz = st[:3]
        ex = st[3] if len(st) > 3 else 2.0
        zo = st[4] if len(st) > 4 else 0.0
        ring = []
        for i in range(n):
            t = 2 * math.pi * i / n
            c, s = math.cos(t), math.sin(t)
            y = ry * math.copysign(abs(c) ** (2 / ex), c)
            z = rz * math.copysign(abs(s) ** (2 / ex), s) + zo
            ring.append(bm.verts.new((x, y, z)))
        rings.append(ring)
    for a, b in zip(rings, rings[1:]):
        for i in range(n):
            j = (i + 1) % n
            bm.faces.new((a[i], a[j], b[j], b[i]))
    bm.faces.new(list(reversed(rings[0])))
    bm.faces.new(rings[-1])
    bmesh.ops.recalc_face_normals(bm, faces=bm.faces[:])
    return mesh_obj(name, bm, color, steps, parent, loc=loc, rot=rot)


def slab(name, pts, z0, z1, color=None, steps=2, parent=None, radius=0.0, loc=(0, 0, 0), rot=(0, 0, 0)):
    """Prisma aus 2D-Umriss [(x, y)] zwischen z0 und z1 (Bildschirme, Karten, Zaehne)."""
    pts = list(pts)
    area = 0.5 * sum(pts[i][0] * pts[(i + 1) % len(pts)][1] - pts[(i + 1) % len(pts)][0] * pts[i][1] for i in range(len(pts)))
    if area < 0:
        pts.reverse()
    bm = bmesh.new()
    vb = [bm.verts.new((x, y, z0)) for x, y in pts]
    vt = [bm.verts.new((x, y, z1)) for x, y in pts]
    bm.faces.new(list(reversed(vb)))
    bm.faces.new(vt)
    for i in range(len(pts)):
        j = (i + 1) % len(pts)
        bm.faces.new((vb[i], vb[j], vt[j], vt[i]))
    o = mesh_obj(name, bm, color, steps, parent, smooth=False, loc=loc, rot=rot)
    if radius > 0:
        bevel(o, radius, 3, 35)
    return o


def rrect(w, h, r, n=8, cx=0.0, cy=0.0):
    r = min(r, w / 2 - 1e-5, h / 2 - 1e-5)
    pts = []
    for x, y, a in ((w / 2 - r, h / 2 - r, 0), (-w / 2 + r, h / 2 - r, 90), (-w / 2 + r, -h / 2 + r, 180), (w / 2 - r, -h / 2 + r, 270)):
        for i in range(n + 1):
            t = math.radians(a + 90 * i / n)
            pts.append((cx + x + r * math.cos(t), cy + y + r * math.sin(t)))
    return pts


def ellipse_pts(rx, ry, n=48, cx=0.0, cy=0.0):
    return [(cx + rx * math.cos(2 * math.pi * i / n), cy + ry * math.sin(2 * math.pi * i / n)) for i in range(n)]


def descendants(o):
    out = []
    for c in o.children:
        out.append(c)
        out += descendants(c)
    return out


def world_points(objs, per_obj=2500):
    dg = bpy.context.evaluated_depsgraph_get()
    dg.update()
    pts = []
    for o in objs:
        if o.type != "MESH" or o.hide_render or o.name.startswith("Schatten"):
            continue
        oe = o.evaluated_get(dg)
        me = oe.to_mesh()
        mw = oe.matrix_world.copy()
        step = max(1, len(me.vertices) // per_obj)
        pts += [mw @ me.vertices[i].co for i in range(0, len(me.vertices), step)]
        oe.to_mesh_clear()
    return pts


def drop_to_floor(root):
    """Gruppe so verschieben, dass ihr tiefster Punkt auf z=0 liegt (nichts schwebt)."""
    pts = world_points(descendants(root) + [root])
    if pts:
        root.location.z -= min(p.z for p in pts)
    bpy.context.view_layer.update()


# ================================================================ Kontur und Schatten

def _kontur_group():
    """Geometry-Nodes-Gruppe: Original + nach aussen versetzte, umgedrehte Kopie mit Kontur-Material.
    (Solidify + Material-Offset wird von Cycles 5.2 hier ignoriert, daher Geometry Nodes.)"""
    g = bpy.data.node_groups.get("KZ_Kontur")
    if g is not None:
        return g
    g = bpy.data.node_groups.new("KZ_Kontur", "GeometryNodeTree")
    g.interface.new_socket("Geometry", in_out="INPUT", socket_type="NodeSocketGeometry")
    g.interface.new_socket("Geometry", in_out="OUTPUT", socket_type="NodeSocketGeometry")
    n, L = g.nodes, g.links
    gi = n.new("NodeGroupInput")
    go = n.new("NodeGroupOutput")
    nor = n.new("GeometryNodeInputNormal")
    wv = n.new("ShaderNodeValue")
    wv.name = "Breite"
    wv.label = "Konturbreite (m)"
    wv.outputs[0].default_value = 0.005
    sc = n.new("ShaderNodeVectorMath")
    sc.operation = "SCALE"
    sp = n.new("GeometryNodeSetPosition")
    fl = n.new("GeometryNodeFlipFaces")
    sm = n.new("GeometryNodeSetMaterial")
    sm.inputs["Material"].default_value = outline_mat()
    jn = n.new("GeometryNodeJoinGeometry")
    L.new(gi.outputs[0], sp.inputs["Geometry"])
    L.new(nor.outputs[0], sc.inputs[0])
    L.new(wv.outputs[0], sc.inputs["Scale"])
    L.new(sc.outputs["Vector"], sp.inputs["Offset"])
    L.new(sp.outputs[0], fl.inputs[0])
    L.new(fl.outputs[0], sm.inputs["Geometry"])
    L.new(sm.outputs[0], jn.inputs[0])
    L.new(gi.outputs[0], jn.inputs[0])
    L.new(jn.outputs[0], go.inputs[0])
    for i, nd in enumerate((gi, nor, sc, sp, fl, sm, jn, go)):
        nd.location = (i * 200 - 700, 0)
    return g


def outline(o, width):
    """Nachtblau-Kontur fuer ein Mesh-Objekt (Inverted Hull per Geometry Nodes). width in m.
    Mehrfach aufrufbar (aendert nur die Breite)."""
    if o.type != "MESH" or o.get("kz_no_outline"):
        return
    g = _kontur_group()
    g.nodes["Breite"].outputs[0].default_value = float(width)   # gilt fuer alle Objekte der Szene
    mod = o.modifiers.get("Kontur")
    if mod is None:
        mod = o.modifiers.new("Kontur", "NODES")
    mod.node_group = g          # auch angehaengte Objekte (Kammi) auf die Szenen-Gruppe umstellen
    o.update_tag()


def outline_all(px=2.2, objs=None):
    """Konturen fuer alle (oder die angegebenen) Meshes, Breite in Pixeln (aus Kamera/Aufloesung)."""
    w = px * pixel_size()
    for o in (objs or SCENE.objects):
        if o.type == "MESH" and not o.name.startswith("Schatten"):
            outline(o, w)
    log("Konturen %.1f px = %.4f m" % (px, w))


def shadow(objs, name="Schatten", alpha=0.12, pad=1.08, zmax=None, center=None, radii=None):
    """Flache Ellipse in Nachtblau (alpha ~12 %) unter Objekten/Figuren auf z=0.
    zmax: nur Punkte unterhalb dieser Hoehe zaehlen (z. B. Fuesse), sonst ganzer Grundriss."""
    if radii is None or center is None:
        allp = []
        for o in objs:
            allp += world_points(descendants(o) + [o])
        if zmax is not None:
            low = [p for p in allp if p.z <= zmax]
            allp = low or allp
        x0, x1 = min(p.x for p in allp), max(p.x for p in allp)
        y0, y1 = min(p.y for p in allp), max(p.y for p in allp)
        center = center or ((x0 + x1) / 2, (y0 + y1) / 2)
        radii = radii or ((x1 - x0) / 2 * pad + 0.02, (y1 - y0) / 2 * pad + 0.02)
    bm = bmesh.new()
    vs = [bm.verts.new((center[0] + radii[0] * math.cos(2 * math.pi * i / 64),
                        center[1] + radii[1] * math.sin(2 * math.pi * i / 64), 0.0008)) for i in range(64)]
    bm.faces.new(vs)
    o = mesh_obj(name, bm, None, smooth=False)
    o.data.materials.append(shadow_mat(alpha))
    o["kz_no_outline"] = 1
    return o


# ================================================================ Render, Licht, Kamera

def clear_scene():
    for o in list(bpy.data.objects):
        bpy.data.objects.remove(o, do_unlink=True)


def setup(res=(1200, 900), samples=24, transparent=True, denoise=False):
    """Cycles CPU, Standard-Farbraum (Palettenfarben kommen exakt heraus), Hintergrund transparent.
    denoise aus: Emission rauscht nicht, Samples dienen nur der Kantenglaettung."""
    sc = SCENE
    sc.render.engine = "CYCLES"
    sc.cycles.device = "CPU"
    sc.cycles.samples = samples
    sc.cycles.preview_samples = 4
    sc.cycles.use_adaptive_sampling = False
    sc.cycles.use_denoising = denoise
    sc.cycles.denoiser = "OPENIMAGEDENOISE"
    sc.cycles.max_bounces = 1
    sc.cycles.diffuse_bounces = 0
    sc.cycles.glossy_bounces = 0
    sc.cycles.transmission_bounces = 0
    sc.cycles.volume_bounces = 0
    sc.cycles.transparent_max_bounces = 24
    sc.cycles.caustics_reflective = False
    sc.cycles.caustics_refractive = False
    sc.cycles.pixel_filter_type = "BLACKMAN_HARRIS"
    sc.cycles.filter_width = 1.5
    sc.render.threads_mode = "FIXED"
    sc.render.threads = 6
    sc.render.resolution_x, sc.render.resolution_y = res
    sc.render.resolution_percentage = 100
    sc.render.film_transparent = transparent
    sc.render.image_settings.file_format = "PNG"
    sc.render.image_settings.color_mode = "RGBA"
    sc.render.image_settings.color_depth = "8"
    sc.view_settings.view_transform = "Standard"
    sc.view_settings.look = "None"
    sc.view_settings.exposure = 0.0
    sc.view_settings.gamma = 1.0
    sc.display_settings.display_device = "sRGB"
    w = bpy.data.worlds.new("kz_welt")
    sc.world = w
    try:
        w.use_nodes = True
    except Exception:
        pass
    bg = w.node_tree.nodes.get("Background")
    if bg:
        bg.inputs["Strength"].default_value = 0.0
    w.color = (1, 1, 1)


def camera(az=-90.0, el=14.0, ortho=True, lens=60.0, target=None, name="Kamera"):
    """Kamera; Richtung von target aus (az, el in Grad). ortho = flacher Illustrationslook."""
    cd = bpy.data.cameras.new(name)
    cd.type = "ORTHO" if ortho else "PERSP"
    cd.lens = lens
    cd.sensor_width = 36
    cd.clip_start = 0.01
    cd.clip_end = 200
    o = bpy.data.objects.new(name, cd)
    link(o)
    SCENE.camera = o
    if target is not None:
        STATE["target"] = Vector(target)
    STATE["cam"] = (az, el)
    _place_cam(o, 12.0)
    return o


def _cam_dir():
    az, el = (math.radians(x) for x in STATE["cam"])
    return Vector((math.cos(az) * math.cos(el), math.sin(az) * math.cos(el), math.sin(el)))


def _place_cam(o, dist):
    d = _cam_dir()
    o.location = STATE["target"] + d * dist
    o.rotation_euler = (-d).to_track_quat("-Z", "Y").to_euler()


def frame(objs=None, fill=0.82, offset=(0.0, 0.0), dist=12.0):
    """Kamera (Ausschnitt) so setzen, dass die Objekte ~fill des Bildes fuellen, zentriert.
    offset verschiebt das Motiv im Bild (Anteil der Bildbreite/-hoehe, + = rechts/oben)."""
    cam = SCENE.camera
    if objs is None:
        objs = [o for o in SCENE.objects if o.type == "MESH" and not o.name.startswith("Schatten")]
    pts = world_points(objs)
    c = Vector(((min(p.x for p in pts) + max(p.x for p in pts)) / 2,
                (min(p.y for p in pts) + max(p.y for p in pts)) / 2,
                (min(p.z for p in pts) + max(p.z for p in pts)) / 2))
    STATE["target"] = c
    rx, ry = SCENE.render.resolution_x, SCENE.render.resolution_y
    big = max(rx, ry)
    cd = cam.data
    cd.shift_x = cd.shift_y = 0.0
    if cd.type == "ORTHO":
        cd.ortho_scale = max(max(p.x for p in pts) - min(p.x for p in pts), max(p.z for p in pts) - min(p.z for p in pts)) * 1.4
    fx, fy = (fill if isinstance(fill, (int, float)) else fill[0]), (fill if isinstance(fill, (int, float)) else fill[1])
    for _ in range(8):
        _place_cam(cam, dist)
        bpy.context.view_layer.update()
        ndc = [world_to_camera_view(SCENE, cam, p) for p in pts]
        xs, ys = [v.x for v in ndc], [v.y for v in ndc]
        ex, ey = max(xs) - min(xs), max(ys) - min(ys)
        sc = max(ex / fx, ey / fy)
        cx, cy = (max(xs) + min(xs)) / 2, (max(ys) + min(ys)) / 2
        cd.shift_x += (cx - 0.5 - offset[0]) * rx / big * (1 if cd.type == "ORTHO" else 1)
        cd.shift_y += (cy - 0.5 - offset[1]) * ry / big
        if cd.type == "ORTHO":
            cd.ortho_scale *= sc
            cd.shift_x /= 1.0
        else:
            dist *= sc
    _place_cam(cam, dist)
    bpy.context.view_layer.update()
    log("Kamera: ortho_scale %.3f shift %.3f %.3f" % (cd.ortho_scale, cd.shift_x, cd.shift_y))


def pixel_size():
    """Weltgroesse eines Pixels am Ziel (fuer Konturbreite)."""
    cam = SCENE.camera
    rx, ry = SCENE.render.resolution_x, SCENE.render.resolution_y
    if cam.data.type == "ORTHO":
        return cam.data.ortho_scale / max(rx, ry)
    d = (cam.location - STATE["target"]).length
    return 2 * d * math.tan(cam.data.angle / 2) / max(rx, ry)


# ================================================================ Viewport (nur Solid!)

def _win():
    return bpy.context.window_manager.windows[0]


def _view3d():
    for area in _win().screen.areas:
        if area.type == "VIEW_3D":
            region = next(r for r in area.regions if r.type == "WINDOW")
            return area, region, area.spaces.active
    return None, None, None


def view_setup():
    area, region, sp = _view3d()
    if sp is None:
        return
    sp.shading.type = "SOLID"
    sp.shading.color_type = "MATERIAL"
    sp.shading.light = "STUDIO"
    sp.shading.show_backface_culling = True   # Konturhuelle nur als Rand (Objekte sind geschlossen)
    sp.overlay.show_floor = True
    sp.clip_start = 0.01
    sp.clip_end = 300
    r3 = sp.region_3d
    r3.view_perspective = "PERSP"
    r3.view_location = STATE["target"]
    r3.view_distance = STATE["size"] * 3.0
    a, e = math.radians(-125), math.radians(22)
    d = Vector((math.cos(a) * math.cos(e), math.sin(a) * math.cos(e), math.sin(e)))
    r3.view_rotation = (-d).to_track_quat("-Z", "Y")
    sp.lens = 50


def view_orbit(deg=1.2):
    if not STATE["orbit"]:
        return
    area, region, sp = _view3d()
    if sp is None:
        return
    r3 = sp.region_3d
    if r3.view_perspective == "CAMERA":
        return
    r3.view_rotation = Quaternion((0, 0, 1), math.radians(deg)) @ r3.view_rotation


def view_flat():
    """Solid ohne Licht = fast schon der flache Endlook (Konturen per Backface Culling sichtbar)."""
    area, region, sp = _view3d()
    if sp is not None:
        sp.shading.light = "FLAT"


def view_camera(overlays=False):
    STATE["orbit"] = False
    area, region, sp = _view3d()
    if sp is None:
        return
    sp.region_3d.view_perspective = "CAMERA"
    sp.overlay.show_overlays = overlays
    try:
        with bpy.context.temp_override(window=_win(), area=area, region=region):
            bpy.ops.view3d.view_center_camera()
    except Exception as e:
        log("view_center_camera:", e)


def header(text):
    area, region, sp = _view3d()
    if area is not None:
        try:
            area.header_text_set(text)
        except Exception:
            pass




# ================================================================ Requisiten (gemeinsam genutzt)
# Alle Requisiten stehen aufrecht, Vorderseite nach -Y, Ursprung = Griffpunkt.

FONT_BOLD = "/usr/share/fonts/noto/NotoSans-Black.ttf"


def text_mesh(name, body, size, color="blau", steps=2, depth=0.01, parent=None, loc=(0, 0, 0), rot=(90, 0, 0),
              font=FONT_BOLD, align="CENTER"):
    """Text als Mesh (damit Toon + Kontur greifen). Standard: aufrecht, zur Kamera (-Y)."""
    cu = bpy.data.curves.new(name, "FONT")
    cu.body = body
    cu.font = bpy.data.fonts.load(font, check_existing=True)
    cu.size = size
    cu.align_x = align
    cu.align_y = "CENTER"
    cu.extrude = depth / 2
    cu.bevel_depth = depth * 0.15
    ob = bpy.data.objects.new(name + "_kurve", cu)
    SCENE.collection.objects.link(ob)
    bpy.context.view_layer.update()
    me = bpy.data.meshes.new_from_object(ob.evaluated_get(bpy.context.evaluated_depsgraph_get()))
    bpy.data.objects.remove(ob, do_unlink=True)
    o = bpy.data.objects.new(name, me)
    o.location = loc
    o.rotation_euler = [math.radians(a) for a in rot]
    link(o, parent)
    me.materials.append(_mat(color, steps))
    return o


def check_mark(name, size, color="weiss", parent=None, loc=(0, 0, 0), radius=None):
    """Haken als Rohr in der XZ-Ebene (zur Kamera), Mitte loc."""
    r = radius or size * 0.11
    pts = [(-0.42, 0.02), (-0.12, -0.3), (0.45, 0.32)]
    return tube(name, [Vector((loc[0] + x * size, loc[1], loc[2] + z * size)) for x, z in pts], r,
                color=flat(color), parent=parent, seg=10)


def prop_phone(name="Handy", check=True, screen="weiss", body="nachtblau"):
    """Smartphone, Display zur Kamera, optional gruener Haken. Ursprung im unteren Drittel (Griff)."""
    root = empty(name, size=0.03)
    rbox(name + "_Gehaeuse", (0.072, 0.011, 0.145), loc=(0, 0, 0.04), radius=0.01, color=body, steps=2, parent=root)
    s = rbox(name + "_Display", (0.062, 0.002, 0.128), loc=(0, -0.0058, 0.04), radius=0.006, color=flat(screen), parent=root)
    s["kz_no_outline"] = 1
    if check:
        d = cylinder(name + "_Kreis", (0, -0.0068, 0.05), 0.021, 0.002, color=flat("mint"), rot=(90, 0, 0), parent=root)
        d["kz_no_outline"] = 1
        check_mark(name + "_Haken", 0.026, "weiss", parent=root, loc=(0, -0.0092, 0.05), radius=0.0026)
        for i, w in enumerate((0.04, 0.028)):
            b = rbox(name + "_Zeile%d" % i, (w, 0.001, 0.006), loc=(0, -0.0068, 0.012 - i * 0.012), radius=0.002,
                     color=flat("hellgrau"), parent=root)
            b["kz_no_outline"] = 1
    return root


def prop_scissors(name="Schere", open_deg=30, blade="hellgrau", ring="koralle"):
    """Friseurschere aufrecht (Spitzen nach oben), Ursprung am Gelenk."""
    root = empty(name, size=0.03)
    for k, sgn in enumerate((1, -1)):
        piv = empty(name + "_Arm%d" % k, rot=(0, sgn * open_deg / 2, 0), parent=root, size=0.01)
        pts = [(-0.006 * sgn, -0.004), (0.007 * sgn, -0.004), (0.0015 * sgn, 0.118), (-0.001 * sgn, 0.118)]
        b = slab(name + "_Klinge%d" % k, [(x, z) for x, z in pts], -0.002 + 0.0022 * k, 0.0002 + 0.0022 * k, color=blade,
                 steps=3, parent=piv, rot=(90, 0, 0))
        tube(name + "_Steg%d" % k, [Vector((0.002 * sgn, 0, -0.004)), Vector((0.006 * sgn, 0, -0.03))], 0.0035,
             color=blade, parent=piv, seg=10)
        ring_pts = [Vector((0.012 * sgn + 0.016 * math.cos(2 * math.pi * i / 24), 0, -0.048 + 0.019 * math.sin(2 * math.pi * i / 24)))
                    for i in range(25)]
        tube(name + "_Ring%d" % k, ring_pts, 0.004, color=ring, steps=2, parent=piv, seg=10, caps=False)
    cylinder(name + "_Schraube", (0, 0.004, 0), 0.0045, 0.004, color="nachtblau", rot=(90, 0, 0), parent=root)
    return root


def prop_calendar(name="Kalender", header="blau", mark="mint", mark2="gelb"):
    """Kalenderblatt mit Kopfband, Ringen und Tageskaesten, ein Tag mit Haken. Ursprung = Blattmitte."""
    root = empty(name, size=0.03)
    rbox(name + "_Blatt", (0.2, 0.008, 0.22), loc=(0, 0, 0), radius=0.01, color="weiss", steps=2, parent=root)
    rbox(name + "_Kopf", (0.2, 0.011, 0.05), loc=(0, -0.0005, 0.085), radius=0.01, color=header, steps=2, parent=root)
    for x in (-0.055, 0.055):
        tube(name + "_Ring%.0f" % (x * 1000), [Vector((x, 0.0, 0.1)), Vector((x, -0.004, 0.125)), Vector((x, 0.0, 0.135))],
             0.0045, color="gelb", parent=root, seg=10)
    k = 0
    for row in range(3):
        for col in range(4):
            x = -0.066 + col * 0.044
            z = 0.025 - row * 0.05
            c = "himmel_hell"
            if (row, col) == (1, 2):
                c = mark
            elif (row, col) == (0, 1):
                c = mark2
            q = rbox(name + "_Tag%d" % k, (0.034, 0.002, 0.038), loc=(x, -0.0045, z), radius=0.005, color=flat(c), parent=root)
            q["kz_no_outline"] = 1
            if c == mark:
                check_mark(name + "_Haken", 0.024, "weiss", parent=root, loc=(x, -0.0062, z), radius=0.0026)
            k += 1
    return root

# ================================================================ Kammi (Maskottchen)
# kammi.blend enthaelt die Collection "Kammi" (siehe kammi_bauen.py). Arme/Fuesse haengen an Empties,
# Posen = Armrichtungen im Rumpf-Raum (Arm zeigt in Ruhe nach -Z), L = Kammis links = +X.

KAMMI_POSES = {
    "steht":    dict(arm_l=(0.32, 0.0, -1), arm_r=(-0.32, 0.0, -1)),
    "winkt":    dict(arm_l=(0.75, -0.2, 0.72), arm_r=(-0.3, -0.08, -1), lean=(0, -4, 0)),
    "handy":    dict(arm_l=(0.3, -0.05, -1), arm_r=(-0.8, -0.55, 0.12), lean=(0, 0, 0)),
    "zeigt":    dict(arm_l=(0.9, -0.35, 0.2), arm_r=(-0.3, -0.05, -1)),
    "schere":   dict(arm_l=(0.62, -0.05, -0.75), arm_r=(-0.6, -0.4, 0.68), lean=(0, 3, 0)),
    "kalender": dict(arm_l=(0.32, -0.1, -1), arm_r=(-0.8, -0.5, 0.22)),
    "traegt":   dict(arm_l=(0.25, -0.95, 0.35), arm_r=(-0.25, -0.95, 0.35)),
    "schlaeft": dict(arm_l=(-0.2, -0.55, -0.8), arm_r=(0.2, -0.55, -0.8), lean=(-8, 9, 0), eyes="zu", sit=True),
    "sitzt":    dict(arm_l=(0.4, -0.6, -0.7), arm_r=(-0.4, -0.6, -0.7), sit=True),
}
KAMMI_PIVOT = (0.0, 0.0, 0.10)
KAMMI_FOOT = 0.082


def kammi_parts(root):
    """Teile eines (ggf. angehaengten, Namen mit .001) Kammi nach Grundnamen."""
    d = {}
    for o in [root] + descendants(root):
        d[o.name.split(".")[0]] = o
    return d


def pose_kammi(root, pose="steht", **over):
    """Pose setzen: Name aus KAMMI_POSES oder eigene Werte (arm_l, arm_r, lean=(x, y, z) Grad, eyes)."""
    p = dict(KAMMI_POSES.get(pose, {}) if isinstance(pose, str) else pose)
    p.update(over)
    parts = kammi_parts(root)
    rest = Vector((0, 0, -1))
    for side in ("l", "r"):
        sh = parts.get("Kammi_Schulter_" + side.upper())
        if sh is None:
            continue
        sh.rotation_mode = "QUATERNION"
        sh.rotation_quaternion = rest.rotation_difference(Vector(p.get("arm_" + side, (0, 0, -1))).normalized())
    rumpf = parts.get("Kammi_Rumpf")
    sit = bool(p.get("sit"))
    if rumpf is not None:
        rumpf.rotation_euler = [math.radians(a) for a in p.get("lean", (0, 0, 0))]
        rumpf.location = (KAMMI_PIVOT[0], KAMMI_PIVOT[1], KAMMI_PIVOT[2] - (0.046 if sit else 0.0))
    for side, sx in (("L", 1), ("R", -1)):
        fe = parts.get("Kammi_Fuss_" + side)
        if fe is None:
            continue
        if sit:     # Beine nach vorn auf dem Boden, Sohlen zur Kamera
            fe.location = (sx * 0.085, -0.16, 0.04)
            fe.rotation_euler = (math.radians(-90), 0, math.radians(sx * -8))
        else:
            fe.location = (sx * KAMMI_FOOT, -0.03, 0.0)
            fe.rotation_euler = (0, 0, 0)
    zu = p.get("eyes", "auf") == "zu"
    for side in ("L", "R"):
        for name, show in (("Kammi_Auge_", not zu), ("Kammi_Glanz_", not zu), ("Kammi_Augezu_", zu)):
            o = parts.get(name + side)
            if o is not None:
                o.hide_render = not show
                o.hide_viewport = not show
    bpy.context.view_layer.update()
    return root


def append_kammi(loc=(0, 0, 0), rot_z=0.0, scale=1.0, pose="steht", **over):
    """Haengt Kammi aus kammi.blend an (beliebig oft), stellt ihn auf den Boden und setzt die Pose.
    Gibt das Wurzel-Empty zurueck. Requisiten: attach(obj, kammi, "R")."""
    before = set(bpy.data.collections)
    with bpy.data.libraries.load(KAMMI_BLEND, link=False) as (src, dst):
        dst.collections = [c for c in src.collections if c == "Kammi"]
    col = [c for c in dst.collections if c is not None][0]
    SCENE.collection.children.link(col)
    root = [o for o in col.objects if o.parent is None and o.name.split(".")[0] == "Kammi"][0]
    root.location = loc
    root.rotation_euler = (0, 0, math.radians(rot_z))
    root.scale = (scale, scale, scale)
    pose_kammi(root, pose, **over)
    set_light(*STATE["light"])
    return root


def attach(obj, kammi_root, side="R", offset=(0, 0, 0), world_rot=(0, 0, 0)):
    """Requisit (Objekt oder Empty mit Kindern) an Kammis Pfote haengen.
    Lage in Weltachsen: offset (m) ab Pfote, world_rot (Grad) = Ausrichtung in der Welt
    (0,0,0 = aufrecht, Vorderseite nach -Y zur Kamera). Bewegt sich danach mit der Pfote."""
    bpy.context.view_layer.update()
    hand = kammi_parts(kammi_root)["Kammi_Hand_" + side.upper()]
    pos = hand.matrix_world.translation + Vector(offset)
    obj.parent = hand
    obj.matrix_parent_inverse.identity()
    obj.matrix_world = Matrix.Translation(pos) @ Euler([math.radians(a) for a in world_rot]).to_matrix().to_4x4()
    bpy.context.view_layer.update()
    return obj

# ================================================================ Ablauf

class Run:
    """Baut die Szene sichtbar Schritt fuer Schritt (build ist ein Generator, jedes yield = Schritt,
    yield <sekunden> = laenger warten), rahmt die Kamera, setzt Konturen, rendert in Cycles,
    speichert PNG (und optional .blend) und beendet Blender.
    frame_fill/offset: Bildausschnitt; frame=False wenn build die Kamera selbst setzt."""

    def __init__(self, name, build, out, res=(1200, 900), samples=24, outline_px=2.2,
                 frame_fill=0.82, offset=(0.0, 0.0), frame=True, cam=(-90.0, 14.0), ortho=True,
                 blend=None, quit=True, light=(-130.0, 48.0)):
        self.name, self.build, self.out = name, build, out
        self.res, self.samples, self.outline_px = res, samples, outline_px
        self.frame_fill, self.offset, self.do_frame = frame_fill, offset, frame
        self.cam, self.ortho, self.blend, self.do_quit, self.light = cam, ortho, blend, quit, light
        self.t_start = time.time()

    def flow(self):
        clear_scene()
        setup(self.res, self.samples)
        STATE["light"] = self.light
        camera(self.cam[0], self.cam[1], self.ortho)
        view_setup()
        header("Kammzeit: %s - Aufbau" % self.name)
        yield 0.8
        n = 0
        for d in self.build():
            n += 1
            yield d
        set_light(*STATE["light"])
        if self.do_frame:
            frame(fill=self.frame_fill, offset=self.offset)
        if self.outline_px:
            outline_all(self.outline_px)
        view_camera(overlays=False)
        view_flat()
        header("Kammzeit: %s - Kamera" % self.name)
        log("Aufbau fertig, %d Schritte, %.1f s" % (n, time.time() - self.t_start))
        yield 1.5

    def tick(self):
        try:
            view_orbit()
            d = next(self.gen)
            return STEP if d is None else d
        except StopIteration:
            self.render()
            return None
        except Exception:
            traceback.print_exc()
            self.quit()
            return None

    def start(self):
        bpy.context.preferences.view.show_splash = False
        bpy.context.preferences.view.render_display_type = "SCREEN"
        try:
            bpy.context.preferences.use_preferences_save = False
        except Exception:
            pass
        self.gen = self.flow()
        bpy.app.timers.register(self.tick, first_interval=1.5)

    def render(self):
        log("Render startet", self.res, "Samples", self.samples)
        header("Kammzeit: %s - Rendern" % self.name)
        bpy.app.handlers.render_complete.append(self._done)
        bpy.app.handlers.render_cancel.append(self._cancel)
        area, region, sp = _view3d()
        self.t0 = time.time()
        with bpy.context.temp_override(window=_win(), area=area, region=region):
            r = bpy.ops.render.render("INVOKE_DEFAULT")
        if "CANCELLED" in r:
            log("INVOKE abgelehnt, rendere blockierend")
            bpy.ops.render.render()
            self._done()

    def _done(self, *a):
        self.t_render = time.time() - self.t0
        log("Render fertig in %.1f s" % self.t_render)
        bpy.app.timers.register(self._save, first_interval=1.0)

    def _cancel(self, *a):
        log("Render abgebrochen")
        bpy.app.timers.register(self.quit, first_interval=0.5)

    def _save(self):
        try:
            os.makedirs(os.path.dirname(self.out), exist_ok=True)
            bpy.data.images["Render Result"].save_render(self.out, scene=SCENE)
            log("PNG gespeichert", self.out, "Renderzeit %.1f s, gesamt %.1f s" % (self.t_render, time.time() - self.t_start))
            if self.blend:
                with bpy.context.temp_override(window=_win()):
                    bpy.ops.wm.save_as_mainfile(filepath=self.blend, check_existing=False, compress=True, copy=True)
                log("Blend gespeichert", self.blend)
        except Exception:
            traceback.print_exc()
        if self.do_quit:
            bpy.app.timers.register(self.quit, first_interval=2.5)
        return None

    def quit(self):
        log("Beende Blender")
        try:
            with bpy.context.temp_override(window=_win()):
                bpy.ops.wm.quit_blender()
        except Exception:
            traceback.print_exc()
            os._exit(0)
        return None
