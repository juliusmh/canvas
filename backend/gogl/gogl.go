package goglbackend

import (
	"fmt"
	"image/color"
	"math"

	"github.com/go-gl/gl/v3.2-core/gl"
	"github.com/tfriedel6/canvas/backend/backendbase"
)

const alphaTexSize = 2048

var zeroes [alphaTexSize]byte

type GoGLBackend struct {
	x, y, w, h     int
	fx, fy, fw, fh float64

	buf       uint32
	shadowBuf uint32
	alphaTex  uint32
	sr        solidShader
	lgr       linearGradientShader
	rgr       radialGradientShader
	ipr       imagePatternShader
	sar       solidAlphaShader
	rgar      radialGradientAlphaShader
	lgar      linearGradientAlphaShader
	ipar      imagePatternAlphaShader
	ir        imageShader
	gauss15r  gaussianShader
	gauss63r  gaussianShader
	gauss127r gaussianShader
	offscr1   offscreenBuffer
	offscr2   offscreenBuffer
	glChan    chan func()

	ptsBuf []float32
}

type offscreenBuffer struct {
	tex              uint32
	w                int
	h                int
	renderStencilBuf uint32
	frameBuf         uint32
	alpha            bool
}

func New(x, y, w, h int) (backendbase.Backend, error) {
	err := gl.Init()
	if err != nil {
		return nil, err
	}

	gl.GetError() // clear error state

	b := &GoGLBackend{
		w:      w,
		h:      h,
		fw:     float64(w),
		fh:     float64(h),
		ptsBuf: make([]float32, 0, 4096),
	}

	err = loadShader(solidVS, solidFS, &b.sr.shaderProgram)
	if err != nil {
		return nil, err
	}
	b.sr.shaderProgram.mustLoadLocations(&b.sr)
	if err = glError(); err != nil {
		return nil, err
	}

	err = loadShader(linearGradientVS, linearGradientFS, &b.lgr.shaderProgram)
	if err != nil {
		return nil, err
	}
	b.lgr.shaderProgram.mustLoadLocations(&b.lgr)
	if err = glError(); err != nil {
		return nil, err
	}

	err = loadShader(radialGradientVS, radialGradientFS, &b.rgr.shaderProgram)
	if err != nil {
		return nil, err
	}
	b.rgr.shaderProgram.mustLoadLocations(&b.rgr)
	if err = glError(); err != nil {
		return nil, err
	}

	err = loadShader(imagePatternVS, imagePatternFS, &b.ipr.shaderProgram)
	if err != nil {
		return nil, err
	}
	b.ipr.shaderProgram.mustLoadLocations(&b.ipr)
	if err = glError(); err != nil {
		return nil, err
	}

	err = loadShader(solidAlphaVS, solidAlphaFS, &b.sar.shaderProgram)
	if err != nil {
		return nil, err
	}
	b.sar.shaderProgram.mustLoadLocations(&b.sar)
	if err = glError(); err != nil {
		return nil, err
	}

	err = loadShader(linearGradientAlphaVS, linearGradientFS, &b.lgar.shaderProgram)
	if err != nil {
		return nil, err
	}
	b.lgar.shaderProgram.mustLoadLocations(&b.lgar)
	if err = glError(); err != nil {
		return nil, err
	}

	err = loadShader(radialGradientAlphaVS, radialGradientAlphaFS, &b.rgar.shaderProgram)
	if err != nil {
		return nil, err
	}
	b.rgar.shaderProgram.mustLoadLocations(&b.rgar)
	if err = glError(); err != nil {
		return nil, err
	}

	err = loadShader(imagePatternAlphaVS, imagePatternAlphaFS, &b.ipar.shaderProgram)
	if err != nil {
		return nil, err
	}
	b.ipar.shaderProgram.mustLoadLocations(&b.ipar)
	if err = glError(); err != nil {
		return nil, err
	}

	err = loadShader(imageVS, imageFS, &b.ir.shaderProgram)
	if err != nil {
		return nil, err
	}
	b.ir.shaderProgram.mustLoadLocations(&b.ir)
	if err = glError(); err != nil {
		return nil, err
	}

	err = loadShader(gaussian15VS, gaussian15FS, &b.gauss15r.shaderProgram)
	if err != nil {
		return nil, err
	}
	b.gauss15r.shaderProgram.mustLoadLocations(&b.gauss15r)
	if err = glError(); err != nil {
		return nil, err
	}

	err = loadShader(gaussian63VS, gaussian63FS, &b.gauss63r.shaderProgram)
	if err != nil {
		return nil, err
	}
	b.gauss63r.shaderProgram.mustLoadLocations(&b.gauss63r)
	if err = glError(); err != nil {
		return nil, err
	}

	err = loadShader(gaussian127VS, gaussian127FS, &b.gauss127r.shaderProgram)
	if err != nil {
		return nil, err
	}
	b.gauss127r.shaderProgram.mustLoadLocations(&b.gauss127r)
	if err = glError(); err != nil {
		return nil, err
	}

	gl.GenBuffers(1, &b.buf)
	if err = glError(); err != nil {
		return nil, err
	}

	gl.GenBuffers(1, &b.shadowBuf)
	if err = glError(); err != nil {
		return nil, err
	}

	gl.ActiveTexture(gl.TEXTURE0)
	gl.GenTextures(1, &b.alphaTex)
	gl.BindTexture(gl.TEXTURE_2D, b.alphaTex)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.ALPHA, alphaTexSize, alphaTexSize, 0, gl.ALPHA, gl.UNSIGNED_BYTE, nil)

	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.Enable(gl.STENCIL_TEST)
	gl.StencilMask(0xFF)
	gl.Clear(gl.STENCIL_BUFFER_BIT)
	gl.StencilOp(gl.KEEP, gl.KEEP, gl.KEEP)
	gl.StencilFunc(gl.EQUAL, 0, 0xFF)

	gl.Enable(gl.SCISSOR_TEST)

	return b, nil
}

// SetBounds updates the bounds of the canvas. This would
// usually be called for example when the window is resized
func (b *GoGLBackend) SetBounds(x, y, w, h int) {
	b.x, b.y = x, y
	b.fx, b.fy = float64(x), float64(y)
	b.w, b.h = w, h
	b.fw, b.fh = float64(w), float64(h)
}

func glError() error {
	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		return fmt.Errorf("GL Error: %x", glErr)
	}
	return nil
}

// Activate makes this GL backend active and sets the viewport. Only
// needs to be called if any other GL code changes the viewport
func (b *GoGLBackend) Activate() {
	// if b.offscreen {
	// 	gli.Viewport(0, 0, int32(cv.w), int32(cv.h))
	// 	cv.enableTextureRenderTarget(&cv.offscrBuf)
	// 	cv.offscrImg.w = cv.offscrBuf.w
	// 	cv.offscrImg.h = cv.offscrBuf.h
	// 	cv.offscrImg.tex = cv.offscrBuf.tex
	// } else {
	gl.Viewport(int32(b.x), int32(b.y), int32(b.w), int32(b.h))
	b.disableTextureRenderTarget()
	// }
	// b.applyScissor()
	gl.Clear(gl.STENCIL_BUFFER_BIT)
}

type glColor struct {
	r, g, b, a float64
}

func colorGoToGL(c color.RGBA) glColor {
	var glc glColor
	glc.r = float64(c.R) / 255
	glc.g = float64(c.G) / 255
	glc.b = float64(c.B) / 255
	glc.a = float64(c.A) / 255
	return glc
}

func (b *GoGLBackend) useShader(style *backendbase.FillStyle) (vertexLoc uint32) {
	if lg := style.LinearGradient; lg != nil {
		lg := lg.(*LinearGradient)
		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindTexture(gl.TEXTURE_2D, lg.tex)
		gl.UseProgram(b.lgr.ID)
		from := mat(style.FillMatrix).mul(lg.from)
		to := mat(style.FillMatrix).mul(lg.to)
		dir := to.sub(from)
		length := dir.len()
		dir = dir.scale(1 / length)
		gl.Uniform2f(b.lgr.CanvasSize, float32(b.fw), float32(b.fh))
		inv := mat(style.FillMatrix).invert().f32()
		gl.UniformMatrix3fv(b.lgr.Invmat, 1, false, &inv[0])
		gl.Uniform2f(b.lgr.From, float32(from[0]), float32(from[1]))
		gl.Uniform2f(b.lgr.Dir, float32(dir[0]), float32(dir[1]))
		gl.Uniform1f(b.lgr.Len, float32(length))
		gl.Uniform1i(b.lgr.Gradient, 0)
		gl.Uniform1f(b.lgr.GlobalAlpha, float32(style.Color.A)/255)
		return b.lgr.Vertex
	}
	if rg := style.RadialGradient; rg != nil {
		rg := rg.(*RadialGradient)
		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindTexture(gl.TEXTURE_2D, rg.tex)
		gl.UseProgram(b.rgr.ID)
		from := mat(style.FillMatrix).mul(rg.from)
		to := mat(style.FillMatrix).mul(rg.to)
		dir := to.sub(from)
		length := dir.len()
		dir = dir.scale(1 / length)
		gl.Uniform2f(b.rgr.CanvasSize, float32(b.fw), float32(b.fh))
		inv := mat(style.FillMatrix).invert().f32()
		gl.UniformMatrix3fv(b.rgr.Invmat, 1, false, &inv[0])
		gl.Uniform2f(b.rgr.From, float32(from[0]), float32(from[1]))
		gl.Uniform2f(b.rgr.To, float32(to[0]), float32(to[1]))
		gl.Uniform2f(b.rgr.Dir, float32(dir[0]), float32(dir[1]))
		gl.Uniform1f(b.rgr.RadFrom, float32(rg.radFrom))
		gl.Uniform1f(b.rgr.RadTo, float32(rg.radTo))
		gl.Uniform1f(b.rgr.Len, float32(length))
		gl.Uniform1i(b.rgr.Gradient, 0)
		gl.Uniform1f(b.rgr.GlobalAlpha, float32(style.Color.A)/255)
		return b.rgr.Vertex
	}
	if img := style.Image; img != nil {
		img := img.(*Image)
		gl.UseProgram(b.ipr.ID)
		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindTexture(gl.TEXTURE_2D, img.tex)
		gl.Uniform2f(b.ipr.CanvasSize, float32(b.fw), float32(b.fh))
		inv := mat(style.FillMatrix).invert().f32()
		gl.UniformMatrix3fv(b.ipr.Invmat, 1, false, &inv[0])
		gl.Uniform2f(b.ipr.ImageSize, float32(img.w), float32(img.h))
		gl.Uniform1i(b.ipr.Image, 0)
		gl.Uniform1f(b.ipr.GlobalAlpha, float32(style.Color.A)/255)
		return b.ipr.Vertex
	}

	gl.UseProgram(b.sr.ID)
	gl.Uniform2f(b.sr.CanvasSize, float32(b.fw), float32(b.fh))
	c := colorGoToGL(style.Color)
	gl.Uniform4f(b.sr.Color, float32(c.r), float32(c.g), float32(c.b), float32(c.a))
	gl.Uniform1f(b.sr.GlobalAlpha, 1)
	return b.sr.Vertex
}

func (b *GoGLBackend) useAlphaShader(style *backendbase.FillStyle, alphaTexSlot int32) (vertexLoc, alphaTexCoordLoc uint32) {
	if lg := style.LinearGradient; lg != nil {
		lg := lg.(*LinearGradient)
		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindTexture(gl.TEXTURE_2D, lg.tex)
		gl.UseProgram(b.lgar.ID)
		from := mat(style.FillMatrix).mul(lg.from)
		to := mat(style.FillMatrix).mul(lg.to)
		dir := to.sub(from)
		length := dir.len()
		dir = dir.scale(1 / length)
		gl.Uniform2f(b.lgar.CanvasSize, float32(b.fw), float32(b.fh))
		inv := mat(style.FillMatrix).invert().f32()
		gl.UniformMatrix3fv(b.lgar.Invmat, 1, false, &inv[0])
		gl.Uniform2f(b.lgar.From, float32(from[0]), float32(from[1]))
		gl.Uniform2f(b.lgar.Dir, float32(dir[0]), float32(dir[1]))
		gl.Uniform1f(b.lgar.Len, float32(length))
		gl.Uniform1i(b.lgar.Gradient, 0)
		gl.Uniform1i(b.lgar.AlphaTex, alphaTexSlot)
		gl.Uniform1f(b.lgar.GlobalAlpha, float32(style.Color.A)/255)
		return b.lgar.Vertex, b.lgar.AlphaTexCoord
	}
	if rg := style.RadialGradient; rg != nil {
		rg := rg.(*RadialGradient)
		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindTexture(gl.TEXTURE_2D, rg.tex)
		gl.UseProgram(b.rgar.ID)
		from := mat(style.FillMatrix).mul(rg.from)
		to := mat(style.FillMatrix).mul(rg.to)
		dir := to.sub(from)
		length := dir.len()
		dir = dir.scale(1 / length)
		gl.Uniform2f(b.rgar.CanvasSize, float32(b.fw), float32(b.fh))
		inv := mat(style.FillMatrix).invert().f32()
		gl.UniformMatrix3fv(b.rgar.Invmat, 1, false, &inv[0])
		gl.Uniform2f(b.rgar.From, float32(from[0]), float32(from[1]))
		gl.Uniform2f(b.rgar.To, float32(to[0]), float32(to[1]))
		gl.Uniform2f(b.rgar.Dir, float32(dir[0]), float32(dir[1]))
		gl.Uniform1f(b.rgar.RadFrom, float32(rg.radFrom))
		gl.Uniform1f(b.rgar.RadTo, float32(rg.radTo))
		gl.Uniform1f(b.rgar.Len, float32(length))
		gl.Uniform1i(b.rgar.Gradient, 0)
		gl.Uniform1i(b.rgar.AlphaTex, alphaTexSlot)
		gl.Uniform1f(b.rgar.GlobalAlpha, float32(style.Color.A)/255)
		return b.rgar.Vertex, b.rgar.AlphaTexCoord
	}
	if img := style.Image; img != nil {
		img := img.(*Image)
		gl.UseProgram(b.ipar.ID)
		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindTexture(gl.TEXTURE_2D, img.tex)
		gl.Uniform2f(b.ipar.CanvasSize, float32(b.fw), float32(b.fh))
		inv := mat(style.FillMatrix).invert().f32()
		gl.UniformMatrix3fv(b.ipar.Invmat, 1, false, &inv[0])
		gl.Uniform2f(b.ipar.ImageSize, float32(img.w), float32(img.h))
		gl.Uniform1i(b.ipar.Image, 0)
		gl.Uniform1i(b.ipar.AlphaTex, alphaTexSlot)
		gl.Uniform1f(b.ipar.GlobalAlpha, float32(style.Color.A)/255)
		return b.ipar.Vertex, b.ipar.AlphaTexCoord
	}

	gl.UseProgram(b.sar.ID)
	gl.Uniform2f(b.sar.CanvasSize, float32(b.fw), float32(b.fh))
	c := colorGoToGL(style.Color)
	gl.Uniform4f(b.sar.Color, float32(c.r), float32(c.g), float32(c.b), float32(c.a))
	gl.Uniform1i(b.sar.AlphaTex, alphaTexSlot)
	gl.Uniform1f(b.sar.GlobalAlpha, 1)
	return b.sar.Vertex, b.sar.AlphaTexCoord
}

func (b *GoGLBackend) enableTextureRenderTarget(offscr *offscreenBuffer) {
	if offscr.w != b.w || offscr.h != b.h {
		if offscr.w != 0 && offscr.h != 0 {
			gl.DeleteTextures(1, &offscr.tex)
			gl.DeleteFramebuffers(1, &offscr.frameBuf)
			gl.DeleteRenderbuffers(1, &offscr.renderStencilBuf)
		}
		offscr.w = b.w
		offscr.h = b.h

		gl.ActiveTexture(gl.TEXTURE0)
		gl.GenTextures(1, &offscr.tex)
		gl.BindTexture(gl.TEXTURE_2D, offscr.tex)
		// todo do non-power-of-two textures work everywhere?
		if offscr.alpha {
			gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(b.w), int32(b.h), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
		} else {
			gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGB, int32(b.w), int32(b.h), 0, gl.RGB, gl.UNSIGNED_BYTE, nil)
		}
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)

		gl.GenFramebuffers(1, &offscr.frameBuf)
		gl.BindFramebuffer(gl.FRAMEBUFFER, offscr.frameBuf)

		gl.GenRenderbuffers(1, &offscr.renderStencilBuf)
		gl.BindRenderbuffer(gl.RENDERBUFFER, offscr.renderStencilBuf)
		gl.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH24_STENCIL8, int32(b.w), int32(b.h))
		gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_STENCIL_ATTACHMENT, gl.RENDERBUFFER, offscr.renderStencilBuf)

		gl.FramebufferTexture(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, offscr.tex, 0)

		if err := gl.CheckFramebufferStatus(gl.FRAMEBUFFER); err != gl.FRAMEBUFFER_COMPLETE {
			// todo this should maybe not panic
			panic(fmt.Sprintf("Failed to set up framebuffer for offscreen texture: %x", err))
		}

		gl.Clear(gl.COLOR_BUFFER_BIT | gl.STENCIL_BUFFER_BIT)
	} else {
		gl.BindFramebuffer(gl.FRAMEBUFFER, offscr.frameBuf)
	}
}

func (b *GoGLBackend) disableTextureRenderTarget() {
	// if b.offscreen {
	// 	b.enableTextureRenderTarget(&b.offscrBuf)
	// } else {
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	// }
}

type mat [9]float64

func (m mat) invert() mat {
	var identity float64 = 1.0 / (m[0]*m[4]*m[8] + m[3]*m[7]*m[2] + m[6]*m[1]*m[5] - m[6]*m[4]*m[2] - m[3]*m[1]*m[8] - m[0]*m[7]*m[5])

	return mat{
		(m[4]*m[8] - m[5]*m[7]) * identity,
		(m[2]*m[7] - m[1]*m[8]) * identity,
		(m[1]*m[5] - m[2]*m[4]) * identity,
		(m[5]*m[6] - m[3]*m[8]) * identity,
		(m[0]*m[8] - m[2]*m[6]) * identity,
		(m[2]*m[3] - m[0]*m[5]) * identity,
		(m[3]*m[7] - m[4]*m[6]) * identity,
		(m[1]*m[6] - m[0]*m[7]) * identity,
		(m[0]*m[4] - m[1]*m[3]) * identity}
}

func (m mat) f32() [9]float32 {
	return [9]float32{
		float32(m[0]), float32(m[1]), float32(m[2]),
		float32(m[3]), float32(m[4]), float32(m[5]),
		float32(m[6]), float32(m[7]), float32(m[8])}
}

func (m mat) mul(v vec) vec {
	return vec{m[0]*v[0] + m[3]*v[1] + m[6], m[1]*v[0] + m[4]*v[1] + m[7]}
}

type vec [2]float64

func (v1 vec) sub(v2 vec) vec {
	return vec{v1[0] - v2[0], v1[1] - v2[1]}
}

func (v vec) len() float64 {
	return math.Sqrt(v[0]*v[0] + v[1]*v[1])
}

func (v vec) scale(f float64) vec {
	return vec{v[0] * f, v[1] * f}
}